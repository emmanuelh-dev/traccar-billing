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
	// GetTenantByOwner finds the books of one Traccar user on one server.
	// Looking a tenant up by base URL alone is what used to hand every user
	// of a server the same books.
	GetTenantByOwner(ctx context.Context, baseURL string, traccarUserID int64) (Tenant, error)
	UpdateTenantSession(ctx context.Context, tenantID int64, session Session) error
	// UpdateTenantOwner refreshes what the tenant knows about the person who
	// just signed in: their Traccar user id, which SetUserDisabled refuses to
	// pause, and the email shown in the header.
	UpdateTenantOwner(ctx context.Context, tenantID int64, traccarUserID int64, email string) error
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

	ListPaymentItems(ctx context.Context, paymentID int64) ([]PaymentItem, error)
	// ReplacePaymentItems clears existing items for a payment and inserts the new ones,
	// setting Position based on slice index, and returning the persisted items with IDs.
	ReplacePaymentItems(ctx context.Context, paymentID int64, items []PaymentItem) ([]PaymentItem, error)

	CreateExpense(ctx context.Context, e Expense) (Expense, error)
	UpdateExpense(ctx context.Context, e Expense) (Expense, error)
	GetExpense(ctx context.Context, tenantID, expenseID int64) (Expense, error)
	// ListExpenses retrieves expenses for a tenant, applying From/To date filters on spent_at
	// while ignoring AccountID in PaymentFilter.
	ListExpenses(ctx context.Context, tenantID int64, filter PaymentFilter) ([]Expense, error)
	DeleteExpense(ctx context.Context, tenantID, expenseID int64) error

	CreateAppointment(ctx context.Context, a Appointment) (Appointment, error)
	UpdateAppointment(ctx context.Context, a Appointment) (Appointment, error)
	GetAppointment(ctx context.Context, tenantID, appointmentID int64) (Appointment, error)
	ListAppointments(ctx context.Context, tenantID int64, filter AppointmentFilter) ([]Appointment, error)
	// SetAppointmentStatus closes or cancels a visit, stamping ClosedAt when it
	// leaves the scheduled state and recording why in Outcome.
	SetAppointmentStatus(ctx context.Context, tenantID, appointmentID int64, status AppointmentStatus, outcome string) (Appointment, error)
	DeleteAppointment(ctx context.Context, tenantID, appointmentID int64) error

	// CreateRemission returns ErrConflict when the subscription already has a
	// remission for that period, which is how re-running the month-end job stays
	// idempotent.
	CreateRemission(ctx context.Context, r Remission) (Remission, error)
	GetRemission(ctx context.Context, tenantID, remissionID int64) (Remission, error)
	ListRemissions(ctx context.Context, tenantID int64, filter RemissionFilter) ([]TenantRemission, error)
	// SettleRemission links the remission to the payment that paid it.
	SettleRemission(ctx context.Context, tenantID, remissionID, paymentID int64, paidAt time.Time) (Remission, error)
	CancelRemission(ctx context.Context, tenantID, remissionID int64, canceledAt time.Time) error
}

type PaymentFilter struct {
	From      time.Time
	To        time.Time
	AccountID int64
}

type TenantPayment struct {
	Payment
	AccountName      string
	ConceptName      string
	ConceptRecurring bool
}

type RemissionFilter struct {
	From      time.Time
	To        time.Time // filter on period_start
	AccountID int64
	Status    RemissionStatus // empty means all
}

type TenantRemission struct {
	Remission
	AccountName string
}
