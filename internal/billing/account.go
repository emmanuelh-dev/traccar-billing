package billing

import "time"

type Account struct {
	ID            int64
	TenantID      int64
	TraccarUserID int64
	Name          string
	Email         string
	DeviceCount   int
	SellerID      int64
	ArchivedAt    time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (a Account) Archived() bool {
	return !a.ArchivedAt.IsZero()
}
