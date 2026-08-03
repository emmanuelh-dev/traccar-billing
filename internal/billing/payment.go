package billing

import "time"

type Payment struct {
	ID             int64
	SubscriptionID int64
	AmountCents    int64
	Currency       string
	PaidAt         time.Time
	Note           string
	CreatedAt      time.Time
}
