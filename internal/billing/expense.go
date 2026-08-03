package billing

import (
	"strings"
	"time"
)

type Expense struct {
	ID          int64
	TenantID    int64
	SellerID    int64
	Category    string
	AmountCents int64
	Currency    string
	SpentAt     time.Time
	Method      string
	Reference   string
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (e Expense) Normalized() Expense {
	e.Category = strings.TrimSpace(e.Category)
	e.Currency = strings.ToUpper(strings.TrimSpace(e.Currency))
	if e.Currency == "" {
		e.Currency = "MXN"
	}
	e.Method = strings.TrimSpace(e.Method)
	e.Reference = strings.TrimSpace(e.Reference)
	e.Note = strings.TrimSpace(e.Note)
	return e
}
