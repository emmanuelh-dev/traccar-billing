package billing

import (
	"strings"
	"time"
)

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

// Mirror reports whether this account is one of Traccar's temporary
// device-share users rather than a real customer. Traccar mints them with
// a composite email of the form "owner@example.com:867...", reusing the
// owner's address plus the shared device's unique ID, and gives them an
// expiration date. They mirror a device another account already pays for,
// so they must never be billed or listed alongside real accounts.
func (a Account) Mirror() bool {
	return strings.Contains(a.Email, ":")
}
