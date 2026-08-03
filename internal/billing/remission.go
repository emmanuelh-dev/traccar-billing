package billing

import "time"

type RemissionStatus string

const (
	RemissionPending  RemissionStatus = "pending"
	RemissionPaid     RemissionStatus = "paid"
	RemissionCanceled RemissionStatus = "canceled"
)

type Remission struct {
	ID             int64
	TenantID       int64
	AccountID      int64
	SubscriptionID int64
	PeriodStart    time.Time
	PeriodEnd      time.Time
	DeviceCount    int
	AmountCents    int64
	Currency       string
	Status         RemissionStatus
	Note           string
	PaymentID      int64 // 0 when unpaid
	IssuedAt       time.Time
	PaidAt         time.Time // zero when unpaid
	CanceledAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PeriodFor returns the first instant of now's month and the last day of that
// same month, both at 00:00 in now's location.
func PeriodFor(now time.Time) (start, end time.Time) {
	year, month, _ := now.Date()
	loc := now.Location()
	start = time.Date(year, month, 1, 0, 0, 0, 0, loc)
	end = time.Date(year, month+1, 0, 0, 0, 0, 0, loc)
	return start, end
}

// BuildRemission freezes the device count and charge amount for the target period.
func BuildRemission(sub Subscription, acc Account, period time.Time) Remission {
	start, end := PeriodFor(period)
	deviceCount := BillableDevices(sub, acc.DeviceCount)
	amountCents := ChargeCents(sub, acc.DeviceCount)

	return Remission{
		TenantID:       acc.TenantID,
		AccountID:      acc.ID,
		SubscriptionID: sub.ID,
		PeriodStart:    start,
		PeriodEnd:      end,
		DeviceCount:    deviceCount,
		AmountCents:    amountCents,
		Currency:       sub.Currency,
		Status:         RemissionPending,
		IssuedAt:       start,
	}
}

// ShouldBill reports whether a subscription is eligible for month-end billing.
func ShouldBill(sub Subscription) bool {
	return sub.Calendar() && sub.Status != StatusCanceled
}

func (r Remission) Pending() bool {
	return r.Status == RemissionPending
}
