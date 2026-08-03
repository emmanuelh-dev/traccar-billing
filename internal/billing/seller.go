package billing

import "time"

type Seller struct {
	ID           int64
	TenantID     int64
	Name         string
	Email        string
	Phone        string
	CommissionBP int
	Active       bool
	Note         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s Seller) CommissionPercent() float64 {
	return float64(s.CommissionBP) / 100
}

func CommissionCents(amountCents int64, commissionBP int) int64 {
	if commissionBP <= 0 || amountCents <= 0 {
		return 0
	}
	return amountCents * int64(commissionBP) / 10000
}
