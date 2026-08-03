package billing

import "time"

// Tenant is one Traccar user's set of books, not one Traccar server. Two
// people who log into the same server get a tenant each, so neither sees the
// other's accounts, payments, expenses or agenda.
type Tenant struct {
	ID      int64
	Name    string
	BaseURL string
	// TraccarUserID identifies the owner. It is the tenant's real identity
	// together with BaseURL, and it is a user id rather than an email so a
	// customer renaming their Traccar login does not orphan their books.
	TraccarUserID      int64
	OwnerEmail         string
	SessionCookie      string
	SessionExpiresAt   time.Time
	AdminTraccarUserID int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (t Tenant) HasValidSession(now time.Time) bool {
	return t.SessionCookie != "" && t.SessionExpiresAt.After(now)
}

type Session struct {
	Cookie    string
	ExpiresAt time.Time
}
