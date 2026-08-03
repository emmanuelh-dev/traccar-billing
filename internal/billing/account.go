package billing

import "time"

type Account struct {
	ID            int64
	TenantID      int64
	TraccarUserID int64
	Name          string
	Email         string
	DeviceCount   int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
