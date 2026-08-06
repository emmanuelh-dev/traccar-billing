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
	TraccarUserID int64
	OwnerEmail    string
	SessionCookie string
	// APIToken is a Traccar token that outlives the login cookie. It is what
	// lets the scheduler keep suspending overdue accounts during the weeks
	// nobody opens the web UI; without it, enforcement stops the moment the
	// cookie expires and nothing says so.
	APIToken             string
	ConnectivityProvider string
	ConnectivityToken    string
	SessionExpiresAt     time.Time
	AdminTraccarUserID   int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// HasValidSession reports whether the tenant can still call Traccar. A token
// has no expiry of its own, so a tenant that holds one stays usable even after
// the login cookie is long dead.
func (t Tenant) HasValidSession(now time.Time) bool {
	if t.APIToken != "" {
		return true
	}
	return t.SessionCookie != "" && t.SessionExpiresAt.After(now)
}

// TraccarSession is how the tenant authenticates against Traccar right now.
func (t Tenant) TraccarSession() Session {
	return Session{Cookie: t.SessionCookie, ExpiresAt: t.SessionExpiresAt, Token: t.APIToken}
}

type Session struct {
	Cookie    string
	ExpiresAt time.Time
	// Token, when set, authenticates instead of Cookie.
	Token string
}
