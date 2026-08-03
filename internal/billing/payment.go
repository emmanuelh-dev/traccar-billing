package billing

import "time"

type Payment struct {
	ID             int64
	AccountID      int64
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
	Items          []PaymentItem
}

type PaymentItem struct {
	ID             int64
	PaymentID      int64
	ConceptID      int64
	Description    string
	Quantity       int
	UnitPriceCents int64
	AmountCents    int64
	Position       int
	CreatedAt      time.Time
}

// Total prefers an explicit AmountCents so an operator can override a line
// with a negotiated price or a discount; otherwise it multiplies out.
func (i PaymentItem) Total() int64 {
	if i.AmountCents != 0 {
		return i.AmountCents
	}
	return int64(i.Quantity) * i.UnitPriceCents
}

func ItemsTotal(items []PaymentItem) int64 {
	var total int64
	for _, item := range items {
		total += item.Total()
	}
	return total
}

func (p Payment) Voided() bool {
	return !p.VoidedAt.IsZero()
}
