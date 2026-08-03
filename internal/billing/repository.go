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
	CreateTenant(ctx context.Context, t Tenant) (Tenant, error)
	GetTenantByID(ctx context.Context, id int64) (Tenant, error)
	GetTenantByBaseURL(ctx context.Context, baseURL string) (Tenant, error)
	UpdateTenantSession(ctx context.Context, tenantID int64, session Session) error
	UpdateTenantAdmin(ctx context.Context, tenantID int64, adminTraccarUserID int64) error
	ListTenants(ctx context.Context) ([]Tenant, error)

	UpsertAccount(ctx context.Context, a Account) (Account, error)
	GetAccount(ctx context.Context, tenantID, accountID int64) (Account, error)
	ListAccountsByTenant(ctx context.Context, tenantID int64) ([]Account, error)

	UpsertSubscription(ctx context.Context, s Subscription) (Subscription, error)
	GetSubscription(ctx context.Context, id int64) (Subscription, error)
	GetSubscriptionByAccountID(ctx context.Context, accountID int64) (Subscription, error)
	ListSubscriptionsDueBefore(ctx context.Context, tenantID int64, cutoff time.Time) ([]Subscription, error)

	RecordPayment(ctx context.Context, p Payment) (Payment, error)
	ListPaymentsBySubscription(ctx context.Context, subscriptionID int64) ([]Payment, error)
}
