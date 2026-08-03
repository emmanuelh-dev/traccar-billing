package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	ctx := context.Background()

	repo, err := OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	if err := RunMigrations(repo.DB(), "sqlite"); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}
	return repo
}

func TestTenantLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Acme Fleet", BaseURL: "https://acme.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if created.ID == 0 {
		t.Fatal("CreateTenant() returned zero ID")
	}

	byID, err := repo.GetTenantByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTenantByID() error = %v", err)
	}
	if byID.BaseURL != created.BaseURL {
		t.Errorf("GetTenantByID() BaseURL = %q, want %q", byID.BaseURL, created.BaseURL)
	}

	byURL, err := repo.GetTenantByBaseURL(ctx, created.BaseURL)
	if err != nil {
		t.Fatalf("GetTenantByBaseURL() error = %v", err)
	}
	if byURL.ID != created.ID {
		t.Errorf("GetTenantByBaseURL() ID = %d, want %d", byURL.ID, created.ID)
	}

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := repo.UpdateTenantSession(ctx, created.ID, billing.Session{Cookie: "JSESSIONID=xyz", ExpiresAt: expiresAt}); err != nil {
		t.Fatalf("UpdateTenantSession() error = %v", err)
	}

	updated, err := repo.GetTenantByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTenantByID() after update error = %v", err)
	}
	if updated.SessionCookie != "JSESSIONID=xyz" {
		t.Errorf("SessionCookie = %q, want JSESSIONID=xyz", updated.SessionCookie)
	}
	if !updated.HasValidSession(time.Now()) {
		t.Error("HasValidSession() = false, want true after fresh login")
	}

	if err := repo.UpdateTenantSession(ctx, created.ID, billing.Session{}); err != nil {
		t.Fatalf("UpdateTenantSession(clear) error = %v", err)
	}
	cleared, err := repo.GetTenantByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTenantByID() after clear error = %v", err)
	}
	if cleared.HasValidSession(time.Now()) {
		t.Error("HasValidSession() = true, want false after clearing session")
	}

	if err := repo.UpdateTenantAdmin(ctx, created.ID, 42); err != nil {
		t.Fatalf("UpdateTenantAdmin() error = %v", err)
	}
	withAdmin, err := repo.GetTenantByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTenantByID() after UpdateTenantAdmin error = %v", err)
	}
	if withAdmin.AdminTraccarUserID != 42 {
		t.Errorf("AdminTraccarUserID = %d, want 42", withAdmin.AdminTraccarUserID)
	}

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("ListTenants() len = %d, want 1", len(tenants))
	}
}

func TestGetTenantByIDNotFound(t *testing.T) {
	repo := newTestRepo(t)

	_, err := repo.GetTenantByID(context.Background(), 999)
	if !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetTenantByID() error = %v, want billing.ErrNotFound", err)
	}
}

func TestAccountUpsertIsIdempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Acme", BaseURL: "https://acme.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	first, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 42, Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("UpsertAccount() first error = %v", err)
	}

	second, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 42, Name: "Ada Lovelace", Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("UpsertAccount() second error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("UpsertAccount() created a new row instead of updating: first.ID=%d second.ID=%d", first.ID, second.ID)
	}
	if second.Name != "Ada Lovelace" {
		t.Errorf("UpsertAccount() Name = %q, want updated value", second.Name)
	}

	accounts, err := repo.ListAccountsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListAccountsByTenant() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListAccountsByTenant() len = %d, want 1", len(accounts))
	}

	got, err := repo.GetAccount(ctx, tenant.ID, first.ID)
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if got.Email != "ada@example.com" {
		t.Errorf("GetAccount() Email = %q", got.Email)
	}
}

func TestSubscriptionAndPaymentLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Acme", BaseURL: "https://acme.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	account, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 1, Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}

	past := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:   account.ID,
		Status:      billing.StatusActive,
		AmountCents: 5000,
		Currency:    "MXN",
		PeriodDays:  30,
		NextDueAt:   past,
	})
	if err != nil {
		t.Fatalf("UpsertSubscription() create error = %v", err)
	}
	if sub.ID == 0 {
		t.Fatal("UpsertSubscription() returned zero ID")
	}

	due, err := repo.ListSubscriptionsDueBefore(ctx, tenant.ID, time.Now())
	if err != nil {
		t.Fatalf("ListSubscriptionsDueBefore() error = %v", err)
	}
	if len(due) != 1 || due[0].ID != sub.ID {
		t.Fatalf("ListSubscriptionsDueBefore() = %+v, want subscription %d", due, sub.ID)
	}

	paidAt := time.Now().UTC().Truncate(time.Second)
	updatedSub := billing.ApplyPayment(sub, paidAt)
	savedSub, err := repo.UpsertSubscription(ctx, updatedSub)
	if err != nil {
		t.Fatalf("UpsertSubscription() update error = %v", err)
	}
	if savedSub.Status != billing.StatusActive {
		t.Errorf("Status after payment = %v, want active", savedSub.Status)
	}

	payment, err := repo.RecordPayment(ctx, billing.Payment{
		SubscriptionID: sub.ID,
		AmountCents:    5000,
		Currency:       "MXN",
		PaidAt:         paidAt,
		Note:           "manual payment",
	})
	if err != nil {
		t.Fatalf("RecordPayment() error = %v", err)
	}
	if payment.ID == 0 {
		t.Fatal("RecordPayment() returned zero ID")
	}

	payments, err := repo.ListPaymentsBySubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ListPaymentsBySubscription() error = %v", err)
	}
	if len(payments) != 1 || payments[0].Note != "manual payment" {
		t.Fatalf("ListPaymentsBySubscription() = %+v", payments)
	}

	stillDue, err := repo.ListSubscriptionsDueBefore(ctx, tenant.ID, time.Now())
	if err != nil {
		t.Fatalf("ListSubscriptionsDueBefore() after payment error = %v", err)
	}
	if len(stillDue) != 0 {
		t.Errorf("ListSubscriptionsDueBefore() after payment = %+v, want empty", stillDue)
	}
}
