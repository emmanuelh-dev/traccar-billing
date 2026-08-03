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

func seedSubscription(t *testing.T, repo *SQLiteRepository) (billing.Tenant, billing.Account, billing.Subscription) {
	t.Helper()
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Fleet", BaseURL: "https://fleet.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	account, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 5, Name: "Cristian", Email: "c@example.com", DeviceCount: 11})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:      account.ID,
		Status:         billing.StatusActive,
		UnitPriceCents: 20000,
		MinDevices:     2,
		GraceDays:      3,
		Currency:       "MXN",
		PeriodDays:     30,
		NextDueAt:      time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertSubscription() error = %v", err)
	}
	return tenant, account, sub
}

func TestSubscriptionPricingRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	_, _, sub := seedSubscription(t, repo)

	got, err := repo.GetSubscription(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("GetSubscription() error = %v", err)
	}
	if got.UnitPriceCents != 20000 {
		t.Errorf("UnitPriceCents = %d, want 20000", got.UnitPriceCents)
	}
	if got.MinDevices != 2 {
		t.Errorf("MinDevices = %d, want 2", got.MinDevices)
	}
	if got.GraceDays != 3 {
		t.Errorf("GraceDays = %d, want 3", got.GraceDays)
	}
}

func TestPaymentEditAndVoid(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	tenant, account, sub := seedSubscription(t, repo)

	paidAt := time.Now().UTC().Truncate(time.Second)
	created, err := repo.RecordPayment(ctx, billing.Payment{
		SubscriptionID: sub.ID,
		AmountCents:    220000,
		UnitPriceCents: 20000,
		DeviceCount:    11,
		Currency:       "MXN",
		Method:         "cash",
		Reference:      "REF-1",
		PaidAt:         paidAt,
		Note:           "agosto",
	})
	if err != nil {
		t.Fatalf("RecordPayment() error = %v", err)
	}
	if created.DeviceCount != 11 || created.Method != "cash" {
		t.Errorf("RecordPayment() lost detail: %+v", created)
	}

	listed, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListPaymentsByTenant() returned %d rows, want 1", len(listed))
	}
	if listed[0].AccountName != account.Name || listed[0].AccountID != account.ID {
		t.Errorf("ListPaymentsByTenant() account = %d/%q, want %d/%q", listed[0].AccountID, listed[0].AccountName, account.ID, account.Name)
	}

	created.AmountCents = 180000
	created.DeviceCount = 9
	updated, err := repo.UpdatePayment(ctx, created)
	if err != nil {
		t.Fatalf("UpdatePayment() error = %v", err)
	}
	if updated.AmountCents != 180000 || updated.DeviceCount != 9 {
		t.Errorf("UpdatePayment() = %d/%d, want 180000/9", updated.AmountCents, updated.DeviceCount)
	}

	if _, err := repo.GetPayment(ctx, tenant.ID+1, created.ID); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetPayment() across tenants error = %v, want ErrNotFound", err)
	}

	if err := repo.VoidPayment(ctx, created.ID, time.Now().UTC(), "duplicado"); err != nil {
		t.Fatalf("VoidPayment() error = %v", err)
	}
	voided, err := repo.GetPayment(ctx, tenant.ID, created.ID)
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if !voided.Voided() || voided.VoidReason != "duplicado" {
		t.Errorf("VoidPayment() left payment %+v", voided)
	}

	if err := repo.VoidPayment(ctx, created.ID, time.Now().UTC(), "otra vez"); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("VoidPayment() twice error = %v, want ErrNotFound", err)
	}
	if _, err := repo.UpdatePayment(ctx, voided); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("UpdatePayment() on voided error = %v, want ErrNotFound", err)
	}
}

func TestWithTxRollsBack(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	tenant, _, sub := seedSubscription(t, repo)

	wantErr := errors.New("boom")
	err := repo.WithTx(ctx, func(tx billing.Repository) error {
		if _, err := tx.RecordPayment(ctx, billing.Payment{
			SubscriptionID: sub.ID,
			AmountCents:    5000,
			Currency:       "MXN",
			PaidAt:         time.Now().UTC(),
		}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx() error = %v, want %v", err, wantErr)
	}

	listed, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("WithTx() rollback left %d payments", len(listed))
	}
}

func TestListPaymentsByTenantFiltersByPeriod(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	tenant, _, sub := seedSubscription(t, repo)

	july := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	for _, paidAt := range []time.Time{july, august} {
		if _, err := repo.RecordPayment(ctx, billing.Payment{
			SubscriptionID: sub.ID,
			AmountCents:    10000,
			Currency:       "MXN",
			PaidAt:         paidAt,
		}); err != nil {
			t.Fatalf("RecordPayment() error = %v", err)
		}
	}

	augustOnly, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{
		From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(augustOnly) != 1 {
		t.Fatalf("august filter returned %d payments, want 1", len(augustOnly))
	}
	if !augustOnly[0].PaidAt.Equal(august) {
		t.Errorf("august filter returned payment dated %v, want %v", augustOnly[0].PaidAt, august)
	}

	all, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered returned %d payments, want 2", len(all))
	}

	none, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{AccountID: 9999})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown account filter returned %d payments, want 0", len(none))
	}
}

func TestDeleteAccountRemovesSubscriptionAndPayments(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Acme", BaseURL: "https://acme.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	account, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 1, Name: "Prueba", Email: "p@example.com"})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:   account.ID,
		Status:      billing.StatusActive,
		AmountCents: 5000,
		Currency:    "MXN",
		PeriodDays:  30,
		NextDueAt:   time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertSubscription() error = %v", err)
	}
	if _, err := repo.RecordPayment(ctx, billing.Payment{
		SubscriptionID: sub.ID,
		AmountCents:    5000,
		Currency:       "MXN",
		PaidAt:         time.Now().UTC().Truncate(time.Second),
		Method:         "cash",
	}); err != nil {
		t.Fatalf("RecordPayment() error = %v", err)
	}

	if err := repo.DeleteAccount(ctx, tenant.ID, account.ID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if _, err := repo.GetAccount(ctx, tenant.ID, account.ID); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetAccount() after delete error = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetSubscriptionByAccountID(ctx, account.ID); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetSubscriptionByAccountID() after delete error = %v, want ErrNotFound", err)
	}
	payments, err := repo.ListPaymentsBySubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ListPaymentsBySubscription() error = %v", err)
	}
	if len(payments) != 0 {
		t.Errorf("payments left after delete = %d, want 0", len(payments))
	}

	if err := repo.DeleteAccount(ctx, tenant.ID, account.ID); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("DeleteAccount() twice error = %v, want ErrNotFound", err)
	}
}

func TestSettingsDefaultsAndSave(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Acme", BaseURL: "https://acme.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	// A tenant created before the settings table existed still gets a
	// usable set of defaults instead of a zero value.
	got, err := repo.GetSettings(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if got.BillingMode != billing.ModeRolling {
		t.Errorf("default BillingMode = %q, want rolling", got.BillingMode)
	}
	if !got.HideMirrorAccounts {
		t.Error("default HideMirrorAccounts = false, want true")
	}
	if got.Currency != "MXN" || got.PeriodDays != 30 {
		t.Errorf("defaults = %+v", got)
	}

	got.BillingMode = billing.ModeCalendar
	got.UnitPriceCents = 20000
	got.HideMirrorAccounts = false
	got.AnchorDay = 99 // out of range: Normalized() must clamp it back
	saved, err := repo.SaveSettings(ctx, got)
	if err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	if saved.AnchorDay != 1 {
		t.Errorf("AnchorDay = %d, want the clamped default 1", saved.AnchorDay)
	}

	reloaded, err := repo.GetSettings(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetSettings() after save error = %v", err)
	}
	if reloaded.BillingMode != billing.ModeCalendar || reloaded.UnitPriceCents != 20000 || reloaded.HideMirrorAccounts {
		t.Errorf("reloaded settings = %+v", reloaded)
	}
}

func TestConceptsCRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, billing.Tenant{Name: "Fleet", BaseURL: "https://fleet.example.com/api"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	seeded, err := repo.ListConcepts(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListConcepts() error = %v", err)
	}
	if len(seeded) != 3 {
		t.Fatalf("expected 3 seeded concepts, got %d", len(seeded))
	}

	created, err := repo.CreateConcept(ctx, billing.Concept{
		TenantID:    tenant.ID,
		Name:        " Mantenimiento ",
		Slug:        "mantenimiento",
		AmountCents: 15000,
		Currency:    "MXN",
		Recurring:   false,
		Active:      true,
		Note:        "Servicio técnico",
	})
	if err != nil {
		t.Fatalf("CreateConcept() error = %v", err)
	}
	if created.Name != "Mantenimiento" {
		t.Errorf("Name = %q, want trimmed Mantenimiento", created.Name)
	}
	if created.Slug != "mantenimiento" {
		t.Errorf("Slug = %q, want mantenimiento", created.Slug)
	}

	created.Name = "Mantenimiento General"
	updated, err := repo.UpdateConcept(ctx, created)
	if err != nil {
		t.Fatalf("UpdateConcept() error = %v", err)
	}
	if updated.Name != "Mantenimiento General" {
		t.Errorf("Updated Name = %q, want Mantenimiento General", updated.Name)
	}

	_, err = repo.CreateConcept(ctx, billing.Concept{
		TenantID: tenant.ID,
		Name:     "Mantenimiento 2",
		Slug:     "mantenimiento",
	})
	if err == nil {
		t.Fatal("CreateConcept() with duplicate slug should fail, got nil error")
	}

	account, err := repo.UpsertAccount(ctx, billing.Account{TenantID: tenant.ID, TraccarUserID: 10, Name: "Tester", Email: "t@example.com"})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	sub, err := repo.UpsertSubscription(ctx, billing.Subscription{
		AccountID:   account.ID,
		Status:      billing.StatusActive,
		AmountCents: 5000,
		Currency:    "MXN",
		PeriodDays:  30,
		NextDueAt:   time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertSubscription() error = %v", err)
	}

	p, err := repo.RecordPayment(ctx, billing.Payment{
		SubscriptionID: sub.ID,
		ConceptID:      created.ID,
		AmountCents:    15000,
		Currency:       "MXN",
		PaidAt:         time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("RecordPayment() with ConceptID error = %v", err)
	}
	if p.ConceptID != created.ID {
		t.Errorf("Recorded payment ConceptID = %d, want %d", p.ConceptID, created.ID)
	}

	tpList, err := repo.ListPaymentsByTenant(ctx, tenant.ID, billing.PaymentFilter{})
	if err != nil {
		t.Fatalf("ListPaymentsByTenant() error = %v", err)
	}
	if len(tpList) != 1 || tpList[0].ConceptName != "Mantenimiento General" {
		t.Errorf("ListPaymentsByTenant ConceptName = %q, want Mantenimiento General", tpList[0].ConceptName)
	}

	if err := repo.DeleteConcept(ctx, tenant.ID, created.ID); err != nil {
		t.Fatalf("DeleteConcept() error = %v", err)
	}
	gotConcept, err := repo.GetConcept(ctx, tenant.ID, created.ID)
	if err != nil {
		t.Fatalf("GetConcept() after DeleteConcept error = %v", err)
	}
	if gotConcept.Active {
		t.Error("Concept should be deactivated (active=false) when payments exist")
	}

	unused, err := repo.CreateConcept(ctx, billing.Concept{
		TenantID: tenant.ID,
		Name:     "Temporal",
		Slug:     "temporal",
	})
	if err != nil {
		t.Fatalf("CreateConcept(unused) error = %v", err)
	}
	if err := repo.DeleteConcept(ctx, tenant.ID, unused.ID); err != nil {
		t.Fatalf("DeleteConcept(unused) error = %v", err)
	}
	if _, err := repo.GetConcept(ctx, tenant.ID, unused.ID); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetConcept(unused) after delete error = %v, want ErrNotFound", err)
	}
}
