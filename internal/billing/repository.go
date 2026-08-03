package billing

import (
	"context"
	"time"
)

// Repository is implemented by internal/storage. UpdateTenantSession(ctx,
// id, Session{}) is the documented way to invalidate a tenant's stored
// session: used by the scheduler on a Traccar 401 and by the auth
// middleware when a browser session outlives it.
type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error

	CreateTenant(ctx context.Context, t Tenant) (Tenant, error)
	GetTenantByID(ctx context.Context, id int64) (Tenant, error)
	GetTenantByBaseURL(ctx context.Context, baseURL string) (Tenant, error)
	UpdateTenantSession(ctx context.Context, tenantID int64, session Session) error
	UpdateTenantAdmin(ctx context.Context, tenantID int64, adminTraccarUserID int64) error
	ListTenants(ctx context.Context) ([]Tenant, error)

	UpsertAccount(ctx context.Context, a Account) (Account, error)
	GetAccount(ctx context.Context, tenantID, accountID int64) (Account, error)
	ListAccountsByTenant(ctx context.Context, tenantID int64) ([]Account, error)
	ArchiveAccount(ctx context.Context, accountID int64, archivedAt time.Time) error
	// DeleteAccount removes the account together with its subscription and
	// every payment recorded against it. Unlike ArchiveAccount this is not
	// reversible and loses the billing history.
	DeleteAccount(ctx context.Context, tenantID, accountID int64) error
	AssignAccountSeller(ctx context.Context, tenantID, accountID, sellerID int64) error

	// GetSettings returns DefaultSettings(tenantID) when the tenant has no
	// row yet, so callers never have to special-case a missing one.
	GetSettings(ctx context.Context, tenantID int64) (Settings, error)
	SaveSettings(ctx context.Context, s Settings) (Settings, error)

	CreateSeller(ctx context.Context, s Seller) (Seller, error)
	UpdateSeller(ctx context.Context, s Seller) (Seller, error)
	GetSeller(ctx context.Context, tenantID, sellerID int64) (Seller, error)
	ListSellers(ctx context.Context, tenantID int64) ([]Seller, error)

	CreateConcept(ctx context.Context, c Concept) (Concept, error)
	UpdateConcept(ctx context.Context, c Concept) (Concept, error)
	GetConcept(ctx context.Context, tenantID, conceptID int64) (Concept, error)
	ListConcepts(ctx context.Context, tenantID int64) ([]Concept, error)
	// DeleteConcept removes the concept, or merely deactivates it when
	// payments already reference it, so billing history keeps its label.
	DeleteConcept(ctx context.Context, tenantID, conceptID int64) error

	UpsertSubscription(ctx context.Context, s Subscription) (Subscription, error)
	GetSubscription(ctx context.Context, id int64) (Subscription, error)
	GetSubscriptionByAccountID(ctx context.Context, accountID int64) (Subscription, error)
	ListSubscriptionsDueBefore(ctx context.Context, tenantID int64, cutoff time.Time) ([]Subscription, error)

	RecordPayment(ctx context.Context, p Payment) (Payment, error)
	GetPayment(ctx context.Context, tenantID, paymentID int64) (Payment, error)
	UpdatePayment(ctx context.Context, p Payment) (Payment, error)
	VoidPayment(ctx context.Context, paymentID int64, voidedAt time.Time, reason string) error
	DeletePayment(ctx context.Context, tenantID, paymentID int64) error
	ListPaymentsBySubscription(ctx context.Context, subscriptionID int64) ([]Payment, error)
	ListPaymentsByTenant(ctx context.Context, tenantID int64, filter PaymentFilter) ([]TenantPayment, error)
}

type PaymentFilter struct {
	From      time.Time
	To        time.Time
	AccountID int64
}

type TenantPayment struct {
	Payment
	AccountID        int64
	AccountName      string
	ConceptName      string
	ConceptRecurring bool
}
