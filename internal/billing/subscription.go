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
	ID             int64
	AccountID      int64
	Status         SubscriptionStatus
	AmountCents    int64
	UnitPriceCents int64
	FlatFeeCents   int64
	MinDevices     int
	GraceDays      int
	Currency       string
	PeriodDays     int
	LastPaidAt     time.Time
	NextDueAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s Subscription) PerDevice() bool {
	return s.UnitPriceCents > 0
}
