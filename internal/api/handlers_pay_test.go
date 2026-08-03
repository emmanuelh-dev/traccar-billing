package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
	"github.com/yourusername/traccar-billing/internal/storage"
)

// stubTraccarClient stands in for the real Traccar server: paying a
// subscription restores the user's access, and that call must not reach the
// network from a test.
type stubTraccarClient struct {
	// enabled counts SetUserDisabled(.., false) calls, which is how a
	// payment restores access: a one-off charge must not make any.
	enabled int
}

func (*stubTraccarClient) Login(context.Context, *url.URL, string, string) (billing.Session, billing.TraccarUser, error) {
	return billing.Session{}, billing.TraccarUser{}, nil
}

func (*stubTraccarClient) FetchUsers(context.Context, *url.URL, billing.Session) ([]billing.TraccarUser, error) {
	return nil, nil
}

func (*stubTraccarClient) FetchDevices(context.Context, *url.URL, billing.Session) ([]billing.TraccarDevice, error) {
	return nil, nil
}

func (*stubTraccarClient) FetchDevicesForUser(context.Context, *url.URL, billing.Session, int64) ([]billing.TraccarDevice, error) {
	return nil, nil
}

func (*stubTraccarClient) FetchServerInfo(context.Context, *url.URL, billing.Session) (billing.TraccarServerInfo, error) {
	return billing.TraccarServerInfo{}, nil
}

func (c *stubTraccarClient) SetUserDisabled(_ context.Context, _ *url.URL, _ billing.Session, _ int64, disabled bool) error {
	if !disabled {
		c.enabled++
	}
	return nil
}

func (*stubTraccarClient) DeleteUser(context.Context, *url.URL, billing.Session, int64) error {
	return nil
}

func newTestServer(t *testing.T) (*Server, billing.Repository, *stubTraccarClient) {
	t.Helper()
	ctx := context.Background()
	sqliteRepo, err := storage.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() { sqliteRepo.Close() })

	if err := storage.RunMigrations(sqliteRepo.DB(), "sqlite"); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &stubTraccarClient{}
	srv := NewServer(sqliteRepo, client, "secret", time.UTC, logger)
	return srv, sqliteRepo, client
}

func setupTestTenantAndAccount(t *testing.T, repo billing.Repository) (billing.Tenant, billing.Account, billing.Subscription) {
	t.Helper()
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{
		Name:             "Test Fleet",
		BaseURL:          "https://fleet.example.com/api",
		SessionCookie:    "session=abc",
		SessionExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateTenant error = %v", err)
	}

	account, err := repo.UpsertAccount(ctx, billing.Account{
		TenantID:      tenant.ID,
		TraccarUserID: 10,
		Name:          "Cristian Palomo",
		Email:         "cristian@example.com",
		DeviceCount:   11,
	})
	if err != nil {
		t.Fatalf("UpsertAccount error = %v", err)
	}

	initialDue := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	initialPaid := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:      account.ID,
		Status:         billing.StatusActive,
		UnitPriceCents: 20000,
		Currency:       "MXN",
		PeriodDays:     30,
		LastPaidAt:     initialPaid,
		NextDueAt:      initialDue,
	})
	if err != nil {
		t.Fatalf("UpsertSubscription error = %v", err)
	}

	return tenant, account, sub
}

func TestPayAccountNonRecurringConcept(t *testing.T) {
	srv, repo, client := newTestServer(t)
	ctx := context.Background()
	tenant, account, initialSub := setupTestTenantAndAccount(t, repo)

	concepts, err := repo.ListConcepts(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListConcepts error = %v", err)
	}
	var nonRecurringConcept billing.Concept
	for _, c := range concepts {
		if !c.Recurring {
			nonRecurringConcept = c
			break
		}
	}
	if nonRecurringConcept.ID == 0 {
		t.Fatal("no non-recurring concept found")
	}

	form := url.Values{}
	form.Set("concept_id", strconv.FormatInt(nonRecurringConcept.ID, 10))
	form.Set("amount", "500.00")
	form.Set("currency", "MXN")
	form.Set("paid_at", "2026-08-02")
	form.Set("method", "cash")

	req := httptest.NewRequest("POST", "/accounts/"+strconv.FormatInt(account.ID, 10)+"/pay", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cookieVal, err := srv.signer.encode(tenant.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode session cookie error = %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieVal})

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("response status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	// Verify subscription was NOT modified
	subAfter, err := repo.GetSubscriptionByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetSubscriptionByAccountID error = %v", err)
	}

	if !subAfter.NextDueAt.Equal(initialSub.NextDueAt) {
		t.Errorf("NextDueAt = %v, want unchanged %v", subAfter.NextDueAt, initialSub.NextDueAt)
	}
	if !subAfter.LastPaidAt.Equal(initialSub.LastPaidAt) {
		t.Errorf("LastPaidAt = %v, want unchanged %v", subAfter.LastPaidAt, initialSub.LastPaidAt)
	}

	// Verify payment recorded with DeviceCount 0 and UnitPriceCents 0
	payments, err := repo.ListPaymentsBySubscription(ctx, subAfter.ID)
	if err != nil {
		t.Fatalf("ListPaymentsBySubscription error = %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments count = %d, want 1", len(payments))
	}
	p := payments[0]
	if p.DeviceCount != 0 {
		t.Errorf("Payment.DeviceCount = %d, want 0", p.DeviceCount)
	}
	if p.UnitPriceCents != 0 {
		t.Errorf("Payment.UnitPriceCents = %d, want 0", p.UnitPriceCents)
	}
	if p.AmountCents != 50000 {
		t.Errorf("Payment.AmountCents = %d, want 50000", p.AmountCents)
	}
	if p.ConceptID != nonRecurringConcept.ID {
		t.Errorf("Payment.ConceptID = %d, want %d", p.ConceptID, nonRecurringConcept.ID)
	}
	if client.enabled != 0 {
		t.Errorf("a one-off charge restored Traccar access %d time(s), want 0", client.enabled)
	}
}

func TestPayAccountRecurringConcept(t *testing.T) {
	srv, repo, client := newTestServer(t)
	ctx := context.Background()
	tenant, account, initialSub := setupTestTenantAndAccount(t, repo)

	concepts, err := repo.ListConcepts(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListConcepts error = %v", err)
	}
	var recurringConcept billing.Concept
	for _, c := range concepts {
		if c.Recurring {
			recurringConcept = c
			break
		}
	}
	if recurringConcept.ID == 0 {
		t.Fatal("no recurring concept found")
	}

	form := url.Values{}
	form.Set("concept_id", strconv.FormatInt(recurringConcept.ID, 10))
	form.Set("amount", "2200.00")
	form.Set("currency", "MXN")
	form.Set("paid_at", "2026-08-02")
	form.Set("method", "cash")

	req := httptest.NewRequest("POST", "/accounts/"+strconv.FormatInt(account.ID, 10)+"/pay", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cookieVal2, err := srv.signer.encode(tenant.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode session cookie error = %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieVal2})

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("response status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	// Verify subscription WAS renewed
	subAfter, err := repo.GetSubscriptionByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetSubscriptionByAccountID error = %v", err)
	}

	if subAfter.NextDueAt.Equal(initialSub.NextDueAt) {
		t.Errorf("NextDueAt = %v, expected it to advance from %v", subAfter.NextDueAt, initialSub.NextDueAt)
	}
	if subAfter.LastPaidAt.Equal(initialSub.LastPaidAt) {
		t.Errorf("LastPaidAt = %v, expected it to update from %v", subAfter.LastPaidAt, initialSub.LastPaidAt)
	}
	if client.enabled != 1 {
		t.Errorf("a recurring payment restored Traccar access %d time(s), want 1", client.enabled)
	}
}
