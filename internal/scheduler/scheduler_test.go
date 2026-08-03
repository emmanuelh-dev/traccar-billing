package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
	"github.com/yourusername/traccar-billing/internal/traccar"
)

type fakeRepo struct {
	tenants           []billing.Tenant
	accounts          []billing.Account
	subscriptions     []billing.Subscription
	upsertAccountCall int
	sessionUpdates    []billing.Session
	archived          []int64
	deleted           []int64
	settings          map[int64]billing.Settings
}

func (r *fakeRepo) CreateTenant(ctx context.Context, t billing.Tenant) (billing.Tenant, error) {
	return billing.Tenant{}, errors.New("not implemented")
}

func (r *fakeRepo) GetTenantByID(ctx context.Context, id int64) (billing.Tenant, error) {
	for _, t := range r.tenants {
		if t.ID == id {
			return t, nil
		}
	}
	return billing.Tenant{}, billing.ErrNotFound
}

func (r *fakeRepo) GetTenantByBaseURL(ctx context.Context, baseURL string) (billing.Tenant, error) {
	return billing.Tenant{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdateTenantSession(ctx context.Context, tenantID int64, session billing.Session) error {
	r.sessionUpdates = append(r.sessionUpdates, session)
	for i, t := range r.tenants {
		if t.ID == tenantID {
			r.tenants[i].SessionCookie = session.Cookie
			r.tenants[i].SessionExpiresAt = session.ExpiresAt
		}
	}
	return nil
}

func (r *fakeRepo) UpdateTenantAdmin(ctx context.Context, tenantID int64, adminTraccarUserID int64) error {
	for i, t := range r.tenants {
		if t.ID == tenantID {
			r.tenants[i].AdminTraccarUserID = adminTraccarUserID
		}
	}
	return nil
}

func (r *fakeRepo) ListTenants(ctx context.Context) ([]billing.Tenant, error) {
	return r.tenants, nil
}

func (r *fakeRepo) UpsertAccount(ctx context.Context, a billing.Account) (billing.Account, error) {
	r.upsertAccountCall++
	r.accounts = append(r.accounts, a)
	return a, nil
}

func (r *fakeRepo) GetAccount(ctx context.Context, tenantID, accountID int64) (billing.Account, error) {
	for _, a := range r.accounts {
		if a.TenantID == tenantID && a.ID == accountID {
			return a, nil
		}
	}
	return billing.Account{}, billing.ErrNotFound
}

func (r *fakeRepo) ListAccountsByTenant(ctx context.Context, tenantID int64) ([]billing.Account, error) {
	var live []billing.Account
	for _, a := range r.accounts {
		if a.ArchivedAt.IsZero() {
			live = append(live, a)
		}
	}
	return live, nil
}

func (r *fakeRepo) UpsertSubscription(ctx context.Context, s billing.Subscription) (billing.Subscription, error) {
	for i, existing := range r.subscriptions {
		if existing.ID == s.ID {
			r.subscriptions[i] = s
			return s, nil
		}
	}
	r.subscriptions = append(r.subscriptions, s)
	return s, nil
}

func (r *fakeRepo) GetSubscription(ctx context.Context, id int64) (billing.Subscription, error) {
	return billing.Subscription{}, errors.New("not implemented")
}

func (r *fakeRepo) GetSubscriptionByAccountID(ctx context.Context, accountID int64) (billing.Subscription, error) {
	return billing.Subscription{}, errors.New("not implemented")
}

func (r *fakeRepo) ListSubscriptionsDueBefore(ctx context.Context, tenantID int64, cutoff time.Time) ([]billing.Subscription, error) {
	var due []billing.Subscription
	for _, s := range r.subscriptions {
		if s.NextDueAt.Before(cutoff) {
			due = append(due, s)
		}
	}
	return due, nil
}

func (r *fakeRepo) RecordPayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	return billing.Payment{}, errors.New("not implemented")
}

func (r *fakeRepo) ListPaymentsBySubscription(ctx context.Context, subscriptionID int64) ([]billing.Payment, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepo) ArchiveAccount(ctx context.Context, accountID int64, archivedAt time.Time) error {
	for i := range r.accounts {
		if r.accounts[i].ID == accountID {
			r.accounts[i].ArchivedAt = archivedAt
			r.archived = append(r.archived, accountID)
			return nil
		}
	}
	return billing.ErrNotFound
}

func (r *fakeRepo) DeleteAccount(ctx context.Context, tenantID, accountID int64) error {
	for i := range r.accounts {
		if r.accounts[i].ID == accountID {
			r.accounts = append(r.accounts[:i], r.accounts[i+1:]...)
			r.deleted = append(r.deleted, accountID)
			return nil
		}
	}
	return billing.ErrNotFound
}

func (r *fakeRepo) GetSettings(ctx context.Context, tenantID int64) (billing.Settings, error) {
	if s, ok := r.settings[tenantID]; ok {
		return s, nil
	}
	return billing.DefaultSettings(tenantID), nil
}

func (r *fakeRepo) SaveSettings(ctx context.Context, s billing.Settings) (billing.Settings, error) {
	if r.settings == nil {
		r.settings = make(map[int64]billing.Settings)
	}
	s = s.Normalized()
	r.settings[s.TenantID] = s
	return s, nil
}

func (r *fakeRepo) AssignAccountSeller(ctx context.Context, tenantID, accountID, sellerID int64) error {
	return errors.New("not implemented")
}

func (r *fakeRepo) CreateSeller(ctx context.Context, s billing.Seller) (billing.Seller, error) {
	return billing.Seller{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdateSeller(ctx context.Context, s billing.Seller) (billing.Seller, error) {
	return billing.Seller{}, errors.New("not implemented")
}

func (r *fakeRepo) GetSeller(ctx context.Context, tenantID, sellerID int64) (billing.Seller, error) {
	return billing.Seller{}, errors.New("not implemented")
}

func (r *fakeRepo) ListSellers(ctx context.Context, tenantID int64) ([]billing.Seller, error) {
	return nil, nil
}

func (r *fakeRepo) CreateConcept(ctx context.Context, c billing.Concept) (billing.Concept, error) {
	return billing.Concept{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdateConcept(ctx context.Context, c billing.Concept) (billing.Concept, error) {
	return billing.Concept{}, errors.New("not implemented")
}

func (r *fakeRepo) GetConcept(ctx context.Context, tenantID, conceptID int64) (billing.Concept, error) {
	return billing.Concept{}, errors.New("not implemented")
}

func (r *fakeRepo) ListConcepts(ctx context.Context, tenantID int64) ([]billing.Concept, error) {
	return nil, nil
}

func (r *fakeRepo) DeleteConcept(ctx context.Context, tenantID, conceptID int64) error {
	return errors.New("not implemented")
}

func (r *fakeRepo) DeletePayment(ctx context.Context, tenantID, paymentID int64) error {
	return errors.New("not implemented")
}

func (r *fakeRepo) WithTx(ctx context.Context, fn func(billing.Repository) error) error {
	return fn(r)
}

func (r *fakeRepo) GetPayment(ctx context.Context, tenantID, paymentID int64) (billing.Payment, error) {
	return billing.Payment{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdatePayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	return billing.Payment{}, errors.New("not implemented")
}

func (r *fakeRepo) VoidPayment(ctx context.Context, paymentID int64, voidedAt time.Time, reason string) error {
	return errors.New("not implemented")
}

func (r *fakeRepo) ListPaymentsByTenant(ctx context.Context, tenantID int64, filter billing.PaymentFilter) ([]billing.TenantPayment, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepo) ListPaymentItems(ctx context.Context, paymentID int64) ([]billing.PaymentItem, error) {
	return nil, nil
}

func (r *fakeRepo) ReplacePaymentItems(ctx context.Context, paymentID int64, items []billing.PaymentItem) ([]billing.PaymentItem, error) {
	return nil, nil
}

func (r *fakeRepo) CreateExpense(ctx context.Context, e billing.Expense) (billing.Expense, error) {
	return billing.Expense{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdateExpense(ctx context.Context, e billing.Expense) (billing.Expense, error) {
	return billing.Expense{}, errors.New("not implemented")
}

func (r *fakeRepo) GetExpense(ctx context.Context, tenantID, expenseID int64) (billing.Expense, error) {
	return billing.Expense{}, errors.New("not implemented")
}

func (r *fakeRepo) ListExpenses(ctx context.Context, tenantID int64, filter billing.PaymentFilter) ([]billing.Expense, error) {
	return nil, nil
}

func (r *fakeRepo) DeleteExpense(ctx context.Context, tenantID, expenseID int64) error {
	return errors.New("not implemented")
}

type fakeClient struct {
	users         []billing.TraccarUser
	devices       []billing.TraccarDevice
	fetchErr      error
	fetchCalled   int
	disabledCalls []int64
	disableErr    error
	deletedUsers  []int64
	deleteErr     error
}

func (c *fakeClient) Login(ctx context.Context, baseURL *url.URL, email, password string) (billing.Session, billing.TraccarUser, error) {
	return billing.Session{}, billing.TraccarUser{}, errors.New("not implemented")
}

func (c *fakeClient) FetchUsers(ctx context.Context, baseURL *url.URL, session billing.Session) ([]billing.TraccarUser, error) {
	c.fetchCalled++
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	return c.users, nil
}

func (c *fakeClient) FetchDevices(ctx context.Context, baseURL *url.URL, session billing.Session) ([]billing.TraccarDevice, error) {
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	return c.devices, nil
}

func (c *fakeClient) FetchDevicesForUser(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64) ([]billing.TraccarDevice, error) {
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	return c.devices, nil
}

func (c *fakeClient) FetchServerInfo(ctx context.Context, baseURL *url.URL, session billing.Session) (billing.TraccarServerInfo, error) {
	return billing.TraccarServerInfo{}, errors.New("not implemented")
}

func (c *fakeClient) SetUserDisabled(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64, disabled bool) error {
	c.disabledCalls = append(c.disabledCalls, traccarUserID)
	return c.disableErr
}

func (c *fakeClient) DeleteUser(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64) error {
	c.deletedUsers = append(c.deletedUsers, traccarUserID)
	return c.deleteErr
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSyncTenantSkipsWithoutValidSession(t *testing.T) {
	repo := &fakeRepo{}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())

	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api"}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("syncTenant() error = %v", err)
	}
	if client.fetchCalled != 0 {
		t.Errorf("FetchUsers called %d times, want 0 for tenant without a valid session", client.fetchCalled)
	}
	if repo.upsertAccountCall != 0 {
		t.Errorf("UpsertAccount called %d times, want 0", repo.upsertAccountCall)
	}
}

func TestSyncTenantUpsertsAccountsPerUser(t *testing.T) {
	repo := &fakeRepo{}
	client := &fakeClient{
		users: []billing.TraccarUser{
			{ID: 1, Name: "Ada", Email: "ada@example.com"},
			{ID: 2, Name: "Grace", Email: "grace@example.com"},
		},
	}
	s := New(repo, client, time.Minute, silentLogger())

	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=abc", SessionExpiresAt: time.Now().Add(time.Hour)}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("syncTenant() error = %v", err)
	}
	if repo.upsertAccountCall != 2 {
		t.Errorf("UpsertAccount called %d times, want 2", repo.upsertAccountCall)
	}
}

func TestSyncTenantClearsSessionOnUnauthorized(t *testing.T) {
	repo := &fakeRepo{}
	client := &fakeClient{fetchErr: traccar.ErrUnauthorized}
	s := New(repo, client, time.Minute, silentLogger())

	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=expired", SessionExpiresAt: time.Now().Add(time.Hour)}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("syncTenant() error = %v, want nil (unauthorized is handled, not escalated)", err)
	}
	if len(repo.sessionUpdates) != 1 {
		t.Fatalf("expected exactly one session clear, got %d", len(repo.sessionUpdates))
	}
	if repo.sessionUpdates[0].Cookie != "" {
		t.Errorf("expected session to be cleared, got %+v", repo.sessionUpdates[0])
	}
}

func TestCheckOverdueMarksOnlyPastDueActiveSubscriptions(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{
		accounts: []billing.Account{
			{ID: 1, TenantID: 1, TraccarUserID: 101},
			{ID: 2, TenantID: 1, TraccarUserID: 102},
			{ID: 3, TenantID: 1, TraccarUserID: 103},
		},
		subscriptions: []billing.Subscription{
			{ID: 1, AccountID: 1, Status: billing.StatusActive, NextDueAt: now.Add(-time.Hour)},
			{ID: 2, AccountID: 2, Status: billing.StatusActive, NextDueAt: now.Add(time.Hour)},
			{ID: 3, AccountID: 3, Status: billing.StatusCanceled, NextDueAt: now.Add(-time.Hour)},
		},
	}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())
	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=abc", SessionExpiresAt: now.Add(time.Hour)}

	if err := s.checkOverdue(context.Background(), tenant); err != nil {
		t.Fatalf("checkOverdue() error = %v", err)
	}

	var gotStatuses []billing.SubscriptionStatus
	for _, sub := range repo.subscriptions {
		gotStatuses = append(gotStatuses, sub.Status)
	}
	want := []billing.SubscriptionStatus{billing.StatusOverdue, billing.StatusActive, billing.StatusCanceled}
	for i := range want {
		if gotStatuses[i] != want[i] {
			t.Errorf("subscription %d status = %v, want %v", repo.subscriptions[i].ID, gotStatuses[i], want[i])
		}
	}

	if len(client.disabledCalls) != 1 || client.disabledCalls[0] != 101 {
		t.Errorf("SetUserDisabled calls = %v, want exactly [101] (only the newly-overdue account's Traccar user)", client.disabledCalls)
	}
}

func TestCheckOverdueRetriesPauseEveryTick(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{
		accounts: []billing.Account{{ID: 1, TenantID: 1, TraccarUserID: 101}},
		subscriptions: []billing.Subscription{
			{ID: 1, AccountID: 1, Status: billing.StatusOverdue, NextDueAt: now.Add(-24 * time.Hour)},
		},
	}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())
	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=abc", SessionExpiresAt: now.Add(time.Hour)}

	if err := s.checkOverdue(context.Background(), tenant); err != nil {
		t.Fatalf("checkOverdue() error = %v", err)
	}

	if len(client.disabledCalls) != 1 || client.disabledCalls[0] != 101 {
		t.Errorf("SetUserDisabled calls = %v, want [101] even though the subscription was already overdue", client.disabledCalls)
	}
}

func TestCheckOverdueNeverPausesTheTenantsOwnAdminUser(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{
		accounts: []billing.Account{{ID: 1, TenantID: 1, TraccarUserID: 101}},
		subscriptions: []billing.Subscription{
			{ID: 1, AccountID: 1, Status: billing.StatusActive, NextDueAt: now.Add(-time.Hour)},
		},
	}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())
	tenant := billing.Tenant{
		ID: 1, BaseURL: "https://acme.example.com/api",
		SessionCookie: "JSESSIONID=abc", SessionExpiresAt: now.Add(time.Hour),
		AdminTraccarUserID: 101,
	}

	if err := s.checkOverdue(context.Background(), tenant); err != nil {
		t.Fatalf("checkOverdue() error = %v", err)
	}

	if len(client.disabledCalls) != 0 {
		t.Errorf("SetUserDisabled calls = %v, want none: account 101 is the tenant's own admin user", client.disabledCalls)
	}
}

func TestRunOnceContinuesAfterTenantFailure(t *testing.T) {
	repo := &fakeRepo{
		tenants: []billing.Tenant{
			{ID: 1, BaseURL: "https://bad.example.com/api", SessionCookie: "x", SessionExpiresAt: time.Now().Add(time.Hour)},
			{ID: 2, BaseURL: "https://good.example.com/api", SessionCookie: "y", SessionExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	client := &fakeClient{fetchErr: errors.New("boom: network unreachable")}
	s := New(repo, client, time.Minute, silentLogger())

	s.runOnce(context.Background())

	if client.fetchCalled != 2 {
		t.Errorf("FetchUsers called %d times, want 2 (both tenants attempted despite the first failing)", client.fetchCalled)
	}
}

func TestSyncTenantArchivesAccountsGoneFromTraccar(t *testing.T) {
	repo := &fakeRepo{
		accounts: []billing.Account{
			{ID: 10, TenantID: 1, TraccarUserID: 1, Name: "Ada"},
			{ID: 11, TenantID: 1, TraccarUserID: 99, Name: "Borrado"},
		},
	}
	client := &fakeClient{users: []billing.TraccarUser{{ID: 1, Name: "Ada"}}}
	s := New(repo, client, time.Minute, silentLogger())

	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=abc", SessionExpiresAt: time.Now().Add(time.Hour)}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("SyncTenant() error = %v", err)
	}
	if len(repo.archived) != 1 || repo.archived[0] != 11 {
		t.Fatalf("archived = %v, want [11]", repo.archived)
	}
}

func TestSyncTenantDoesNotArchiveOnFetchFailure(t *testing.T) {
	repo := &fakeRepo{
		accounts: []billing.Account{{ID: 10, TenantID: 1, TraccarUserID: 1, Name: "Ada"}},
	}
	client := &fakeClient{fetchErr: errors.New("traccar down")}
	s := New(repo, client, time.Minute, silentLogger())

	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api", SessionCookie: "JSESSIONID=abc", SessionExpiresAt: time.Now().Add(time.Hour)}

	if err := s.SyncTenant(context.Background(), tenant); err == nil {
		t.Fatal("SyncTenant() error = nil, want the fetch failure")
	}
	if len(repo.archived) != 0 {
		t.Fatalf("archived = %v, want none when the user fetch failed", repo.archived)
	}
}
