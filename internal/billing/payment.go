package billing

import "time"

type Payment struct {
	ID             int64
	SubscriptionID int64
	ConceptID      int64
	AmountCents    int64
	UnitPriceCents int64
	DeviceCount    int
	Currency       string
	Method         string
	Reference      string
	PaidAt         time.Time
	Note           string
	VoidedAt       time.Time
	VoidReason     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (p Payment) Voided() bool {
	return !p.VoidedAt.IsZero()
}
