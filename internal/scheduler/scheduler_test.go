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
	remissions        []billing.Remission
	upsertAccountCall int
	sessionUpdates    []billing.Session
	apiTokenUpdates   []string
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

func (r *fakeRepo) GetTenantByOwner(ctx context.Context, baseURL string, traccarUserID int64) (billing.Tenant, error) {
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

func (r *fakeRepo) UpdateTenantOwner(ctx context.Context, tenantID int64, traccarUserID int64, email string) error {
	for i, t := range r.tenants {
		if t.ID == tenantID {
			r.tenants[i].AdminTraccarUserID = traccarUserID
			r.tenants[i].OwnerEmail = email
		}
	}
	return nil
}

func (r *fakeRepo) UpdateTenantAPIToken(ctx context.Context, tenantID int64, token string) error {
	r.apiTokenUpdates = append(r.apiTokenUpdates, token)
	for i, t := range r.tenants {
		if t.ID == tenantID {
			r.tenants[i].APIToken = token
		}
	}
	return nil
}

func (r *fakeRepo) UpdateTenantConnectivity(context.Context, int64, string, string) error {
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
	for _, s := range r.subscriptions {
		if s.AccountID == accountID {
			return s, nil
		}
	}
	return billing.Subscription{}, billing.ErrNotFound
}

func (r *fakeRepo) CreateRemission(ctx context.Context, rem billing.Remission) (billing.Remission, error) {
	for _, existing := range r.remissions {
		if existing.SubscriptionID == rem.SubscriptionID && existing.PeriodStart.Equal(rem.PeriodStart) {
			return billing.Remission{}, billing.ErrConflict
		}
	}
	rem.ID = int64(len(r.remissions) + 1)
	r.remissions = append(r.remissions, rem)
	return rem, nil
}

func (r *fakeRepo) GetRemission(ctx context.Context, tenantID, remissionID int64) (billing.Remission, error) {
	for _, rem := range r.remissions {
		if rem.TenantID == tenantID && rem.ID == remissionID {
			return rem, nil
		}
	}
	return billing.Remission{}, billing.ErrNotFound
}

func (r *fakeRepo) ListRemissions(ctx context.Context, tenantID int64, filter billing.RemissionFilter) ([]billing.TenantRemission, error) {
	var list []billing.TenantRemission
	for _, rem := range r.remissions {
		if rem.TenantID != tenantID {
			continue
		}
		if filter.AccountID > 0 && rem.AccountID != filter.AccountID {
			continue
		}
		if filter.Status != "" && rem.Status != filter.Status {
			continue
		}
		if !filter.From.IsZero() && rem.PeriodStart.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && rem.PeriodStart.After(filter.To) {
			continue
		}
		accountName := ""
		for _, acc := range r.accounts {
			if acc.ID == rem.AccountID {
				accountName = acc.Name
				break
			}
		}
		list = append(list, billing.TenantRemission{
			Remission:   rem,
			AccountName: accountName,
		})
	}
	return list, nil
}

func (r *fakeRepo) SettleRemission(ctx context.Context, tenantID, remissionID, paymentID int64, paidAt time.Time) (billing.Remission, error) {
	for i, rem := range r.remissions {
		if rem.TenantID == tenantID && rem.ID == remissionID {
			r.remissions[i].Status = billing.RemissionPaid
			r.remissions[i].PaymentID = paymentID
			r.remissions[i].PaidAt = paidAt
			return r.remissions[i], nil
		}
	}
	return billing.Remission{}, billing.ErrNotFound
}

func (r *fakeRepo) CancelRemission(ctx context.Context, tenantID, remissionID int64, canceledAt time.Time) error {
	for i, rem := range r.remissions {
		if rem.TenantID == tenantID && rem.ID == remissionID {
			r.remissions[i].Status = billing.RemissionCanceled
			r.remissions[i].CanceledAt = canceledAt
			return nil
		}
	}
	return billing.ErrNotFound
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
	disabledState []bool
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

func (c *fakeClient) CreateToken(ctx context.Context, baseURL *url.URL, session billing.Session, expiresAt time.Time) (string, error) {
	return "fake-token", nil
}

func (c *fakeClient) FetchServerInfo(ctx context.Context, baseURL *url.URL, session billing.Session) (billing.TraccarServerInfo, error) {
	return billing.TraccarServerInfo{}, errors.New("not implemented")
}

func (c *fakeClient) SetUserDisabled(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64, disabled bool) error {
	c.disabledCalls = append(c.disabledCalls, traccarUserID)
	c.disabledState = append(c.disabledState, disabled)
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

	tenant := billing.Tenant{
		ID: 1, BaseURL: "https://acme.example.com/api",
		APIToken: "revoked-token", SessionCookie: "JSESSIONID=expired",
		SessionExpiresAt: time.Now().Add(-time.Hour),
	}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("syncTenant() error = %v, want nil (unauthorized is handled, not escalated)", err)
	}
	if len(repo.sessionUpdates) != 1 {
		t.Fatalf("expected exactly one session clear, got %d", len(repo.sessionUpdates))
	}
	if repo.sessionUpdates[0].Cookie != "" {
		t.Errorf("expected session to be cleared, got %+v", repo.sessionUpdates[0])
	}
	if len(repo.apiTokenUpdates) != 1 || repo.apiTokenUpdates[0] != "" {
		t.Errorf("expected API token to be cleared, got %v", repo.apiTokenUpdates)
	}
}

func TestSyncTenantReenablesActiveUserAfterFailedReset(t *testing.T) {
	repo := &fakeRepo{
		accounts: []billing.Account{
			{ID: 1, TenantID: 1, TraccarUserID: 101, Name: "Ada"},
		},
		subscriptions: []billing.Subscription{
			{ID: 1, AccountID: 1, Status: billing.StatusActive, NextDueAt: time.Now().Add(24 * time.Hour)},
		},
	}
	client := &fakeClient{
		users: []billing.TraccarUser{
			{ID: 101, Name: "Ada", Disabled: true},
		},
	}
	s := New(repo, client, time.Minute, silentLogger())
	tenant := billing.Tenant{
		ID: 1, BaseURL: "https://acme.example.com/api",
		SessionCookie: "JSESSIONID=abc", SessionExpiresAt: time.Now().Add(time.Hour),
	}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("SyncTenant() error = %v", err)
	}
	if len(client.disabledCalls) != 1 || client.disabledCalls[0] != 101 {
		t.Fatalf("SetUserDisabled calls = %v, want [101]", client.disabledCalls)
	}
	if client.disabledState[0] {
		t.Error("SetUserDisabled disabled = true, want false to restore access")
	}
}

func TestSyncTenantDoesNotTouchMatchingAccessState(t *testing.T) {
	repo := &fakeRepo{
		accounts: []billing.Account{
			{ID: 1, TenantID: 1, TraccarUserID: 101, Name: "Ada"},
		},
		subscriptions: []billing.Subscription{
			{ID: 1, AccountID: 1, Status: billing.StatusActive, NextDueAt: time.Now().Add(24 * time.Hour)},
		},
	}
	client := &fakeClient{
		users: []billing.TraccarUser{
			{ID: 101, Name: "Ada", Disabled: false},
		},
	}
	s := New(repo, client, time.Minute, silentLogger())
	tenant := billing.Tenant{
		ID: 1, BaseURL: "https://acme.example.com/api",
		SessionCookie: "JSESSIONID=abc", SessionExpiresAt: time.Now().Add(time.Hour),
	}

	if err := s.SyncTenant(context.Background(), tenant); err != nil {
		t.Fatalf("SyncTenant() error = %v", err)
	}
	if len(client.disabledCalls) != 0 {
		t.Errorf("SetUserDisabled calls = %v, want none", client.disabledCalls)
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

func (r *fakeRepo) CreateAppointment(ctx context.Context, a billing.Appointment) (billing.Appointment, error) {
	return billing.Appointment{}, errors.New("not implemented")
}

func (r *fakeRepo) UpdateAppointment(ctx context.Context, a billing.Appointment) (billing.Appointment, error) {
	return billing.Appointment{}, errors.New("not implemented")
}

func (r *fakeRepo) GetAppointment(ctx context.Context, tenantID, appointmentID int64) (billing.Appointment, error) {
	return billing.Appointment{}, errors.New("not implemented")
}

func (r *fakeRepo) ListAppointments(ctx context.Context, tenantID int64, filter billing.AppointmentFilter) ([]billing.Appointment, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepo) SetAppointmentStatus(ctx context.Context, tenantID, appointmentID int64, status billing.AppointmentStatus, outcome string) (billing.Appointment, error) {
	return billing.Appointment{}, errors.New("not implemented")
}

func (r *fakeRepo) DeleteAppointment(ctx context.Context, tenantID, appointmentID int64) error {
	return errors.New("not implemented")
}

func TestGenerateRemissions(t *testing.T) {
	now := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	tenant := billing.Tenant{ID: 1, BaseURL: "https://acme.example.com/api"}

	repo := &fakeRepo{
		accounts: []billing.Account{
			{ID: 1, TenantID: 1, Name: "Calendar Active", DeviceCount: 4},
			{ID: 2, TenantID: 1, Name: "Rolling Active", DeviceCount: 2},
			{ID: 3, TenantID: 1, Name: "Calendar Canceled", DeviceCount: 3},
			{ID: 4, TenantID: 1, Name: "Archived Calendar", DeviceCount: 5, ArchivedAt: now.Add(-time.Hour)},
			{ID: 5, TenantID: 1, Name: "Mirror Calendar", Email: "owner@example.com:8675309", DeviceCount: 1},
		},
		subscriptions: []billing.Subscription{
			{ID: 10, AccountID: 1, BillingMode: billing.ModeCalendar, Status: billing.StatusActive, UnitPriceCents: 1000, Currency: "MXN"},
			{ID: 20, AccountID: 2, BillingMode: billing.ModeRolling, Status: billing.StatusActive, UnitPriceCents: 1000, Currency: "MXN"},
			{ID: 30, AccountID: 3, BillingMode: billing.ModeCalendar, Status: billing.StatusCanceled, UnitPriceCents: 1000, Currency: "MXN"},
			{ID: 40, AccountID: 4, BillingMode: billing.ModeCalendar, Status: billing.StatusActive, UnitPriceCents: 1000, Currency: "MXN"},
			{ID: 50, AccountID: 5, BillingMode: billing.ModeCalendar, Status: billing.StatusActive, UnitPriceCents: 1000, Currency: "MXN"},
		},
	}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())

	// 1. Generates exactly one remission per calendar subscription (skipping rolling, canceled, archived, mirror)
	n, err := s.GenerateRemissions(context.Background(), tenant, now)
	if err != nil {
		t.Fatalf("GenerateRemissions error = %v", err)
	}
	if n != 1 {
		t.Errorf("GenerateRemissions created count = %d, want 1", n)
	}
	if len(repo.remissions) != 1 {
		t.Fatalf("len(repo.remissions) = %d, want 1", len(repo.remissions))
	}
	rem := repo.remissions[0]
	if rem.SubscriptionID != 10 {
		t.Errorf("Remission SubscriptionID = %d, want 10", rem.SubscriptionID)
	}
	if rem.AmountCents != 4000 {
		t.Errorf("Remission AmountCents = %d, want 4000 (4 * 1000)", rem.AmountCents)
	}

	// 2. Running it TWICE for the same month creates no duplicates (duplicate-run test)
	n2, err := s.GenerateRemissions(context.Background(), tenant, now)
	if err != nil {
		t.Fatalf("GenerateRemissions second run error = %v", err)
	}
	if n2 != 0 {
		t.Errorf("GenerateRemissions second run created count = %d, want 0", n2)
	}
	if len(repo.remissions) != 1 {
		t.Errorf("len(repo.remissions) after second run = %d, want 1", len(repo.remissions))
	}

	// 3. Amount is frozen: change the account's DeviceCount, re-read, remission amount must be unchanged
	repo.accounts[0].DeviceCount = 10
	remFetched, err := repo.GetRemission(context.Background(), tenant.ID, rem.ID)
	if err != nil {
		t.Fatalf("GetRemission error = %v", err)
	}
	if remFetched.AmountCents != 4000 || remFetched.DeviceCount != 4 {
		t.Errorf("Remission after account change has amount %d count %d, want frozen 4000 and 4", remFetched.AmountCents, remFetched.DeviceCount)
	}

	// 4. Crossing into next month generates a new one
	nextMonth := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)
	n3, err := s.GenerateRemissions(context.Background(), tenant, nextMonth)
	if err != nil {
		t.Fatalf("GenerateRemissions next month error = %v", err)
	}
	if n3 != 1 {
		t.Errorf("GenerateRemissions next month created count = %d, want 1", n3)
	}
	if len(repo.remissions) != 2 {
		t.Errorf("len(repo.remissions) after next month = %d, want 2", len(repo.remissions))
	}
}

func TestGenerateRemissionsLeavesSubscriptionUntouched(t *testing.T) {
	now := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	dueFuture := now.Add(15 * 24 * time.Hour)
	lastPaid := now.Add(-15 * 24 * time.Hour)

	tenant := billing.Tenant{
		ID:                 1,
		BaseURL:            "https://acme.example.com/api",
		SessionCookie:      "JSESSIONID=abc",
		SessionExpiresAt:   now.Add(time.Hour),
		AdminTraccarUserID: 999,
	}

	acc := billing.Account{ID: 1, TenantID: 1, TraccarUserID: 101, Name: "Test Account", DeviceCount: 2}
	sub := billing.Subscription{
		ID:          10,
		AccountID:   1,
		Status:      billing.StatusActive,
		BillingMode: billing.ModeCalendar,
		AmountCents: 2000,
		Currency:    "MXN",
		LastPaidAt:  lastPaid,
		NextDueAt:   dueFuture,
	}

	repo := &fakeRepo{
		tenants:       []billing.Tenant{tenant},
		accounts:      []billing.Account{acc},
		subscriptions: []billing.Subscription{sub},
	}
	client := &fakeClient{}
	s := New(repo, client, time.Minute, silentLogger())

	subBefore := repo.subscriptions[0]

	// Run GenerateRemissions
	if _, err := s.GenerateRemissions(context.Background(), tenant, now); err != nil {
		t.Fatalf("GenerateRemissions error = %v", err)
	}

	subAfter := repo.subscriptions[0]

	// Assert subscription row is byte-for-byte unchanged
	if subAfter != subBefore {
		t.Errorf("Subscription changed after GenerateRemissions!\nBefore: %+v\nAfter:  %+v", subBefore, subAfter)
	}

	// Assert checkOverdue did not disable Traccar user
	if err := s.checkOverdue(context.Background(), tenant); err != nil {
		t.Fatalf("checkOverdue error = %v", err)
	}
	if len(client.disabledCalls) != 0 {
		t.Errorf("checkOverdue called SetUserDisabled for %v, want none", client.disabledCalls)
	}
}
