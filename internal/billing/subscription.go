package billing

import "time"

type SubscriptionStatus string

const (
	StatusActive    SubscriptionStatus = "active"
	StatusOverdue   SubscriptionStatus = "overdue"
	StatusSuspended SubscriptionStatus = "suspended"
	StatusCanceled  SubscriptionStatus = "canceled"
)

type Subscription struct {
	ID          int64
	AccountID   int64
	Status      SubscriptionStatus
	AmountCents int64
	Currency    string
	PeriodDays  int
	LastPaidAt  time.Time
	NextDueAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
