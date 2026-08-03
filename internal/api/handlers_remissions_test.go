package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// seedRemission builds a tenant with one priced account and one pending
// remission, which is the state the month-end run leaves behind.
func seedRemission(t *testing.T, srv *Server, repo billing.Repository) ([]*http.Cookie, billing.Remission, billing.Subscription) {
	t.Helper()
	ctx := context.Background()

	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"owner@example.com": {ID: 7, Name: "Owner", Email: "owner@example.com"},
	}}
	cookies := loginAs(t, srv.Router(), "https://gps.example.com", "owner@example.com")

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	tenant := tenants[0]

	account, err := repo.UpsertAccount(ctx, billing.Account{
		TenantID: tenant.ID, TraccarUserID: 42, Name: "Cliente Uno", Email: "uno@example.com", DeviceCount: 3,
	})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}

	due := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:   account.ID,
		Status:      billing.StatusOverdue,
		BillingMode: billing.ModeCalendar,
		DueDay:      5,
		AmountCents: 30000,
		Currency:    "MXN",
		NextDueAt:   due,
	})
	if err != nil {
		t.Fatalf("UpsertSubscription() error = %v", err)
	}

	rem, err := repo.CreateRemission(ctx, billing.Remission{
		TenantID:       tenant.ID,
		AccountID:      account.ID,
		SubscriptionID: sub.ID,
		PeriodStart:    time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		DeviceCount:    3,
		AmountCents:    30000,
		Currency:       "MXN",
		Status:         billing.RemissionPending,
		IssuedAt:       time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateRemission() error = %v", err)
	}

	return cookies, rem, sub
}

func TestRemissionsPageShowsPendingRemission(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, _, _ := seedRemission(t, srv, repo)

	body := get(t, srv.Router(), "/remissions", cookies).Body.String()

	if !strings.Contains(body, "Cliente Uno") {
		t.Error("the remissions page does not list the account")
	}
	if !strings.Contains(body, "300.00") {
		t.Errorf("the frozen amount is not on the page:\n%s", body)
	}
}

// TestSettleRemissionAdvancesSubscription is the point of the button. A
// remission marked paid whose subscription never moved would be picked up as
// overdue on the scheduler's next tick and the customer suspended despite
// having paid.
func TestSettleRemissionAdvancesSubscription(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, rem, subBefore := seedRemission(t, srv, repo)
	ctx := context.Background()

	form := strings.NewReader("method=cash&reference=recibo-1")
	req := httptest.NewRequest(http.MethodPost, "/remissions/"+strconv.FormatInt(rem.ID, 10)+"/pay", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("settle status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}

	settled, err := repo.GetRemission(ctx, tenants[0].ID, rem.ID)
	if err != nil {
		t.Fatalf("GetRemission() error = %v", err)
	}
	if settled.Status != billing.RemissionPaid {
		t.Errorf("Status = %q, want paid", settled.Status)
	}
	if settled.PaymentID == 0 {
		t.Error("settled remission has no payment linked to it")
	}

	subAfter, err := repo.GetSubscription(ctx, subBefore.ID)
	if err != nil {
		t.Fatalf("GetSubscription() error = %v", err)
	}
	if !subAfter.NextDueAt.After(subBefore.NextDueAt) {
		t.Errorf("NextDueAt = %v, want it moved past %v so the scheduler stops treating it as overdue",
			subAfter.NextDueAt, subBefore.NextDueAt)
	}
	if subAfter.Status != billing.StatusActive {
		t.Errorf("Status = %q, want active after payment", subAfter.Status)
	}

	payments, err := repo.ListPaymentsBySubscription(ctx, subBefore.ID)
	if err != nil {
		t.Fatalf("ListPaymentsBySubscription() error = %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("len(payments) = %d, want 1", len(payments))
	}
	if payments[0].AmountCents != 30000 {
		t.Errorf("payment amount = %d, want the remission's frozen 30000", payments[0].AmountCents)
	}
}

func TestSettleRemissionTwiceIsRejected(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, rem, _ := seedRemission(t, srv, repo)
	handler := srv.Router()
	path := "/remissions/" + strconv.FormatInt(rem.ID, 10) + "/pay"

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	post()
	post()

	tenants, err := repo.ListTenants(context.Background())
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	settled, err := repo.GetRemission(context.Background(), tenants[0].ID, rem.ID)
	if err != nil {
		t.Fatalf("GetRemission() error = %v", err)
	}

	// Paying twice must not double-advance the subscription or record a
	// second payment for money that only arrived once.
	payments, err := repo.ListPaymentsBySubscription(context.Background(), settled.SubscriptionID)
	if err != nil {
		t.Fatalf("ListPaymentsBySubscription() error = %v", err)
	}
	if len(payments) != 1 {
		t.Errorf("len(payments) = %d, want 1: the second click must be refused", len(payments))
	}
}
