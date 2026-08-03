package billing

import (
	"context"
	"net/url"
	"time"
)

// TraccarClient is stateless: every method takes the target server's base
// URL and Session explicitly, so one client value can serve every tenant
// concurrently with no per-tenant state or locking.
type TraccarClient interface {
	// Login also returns the authenticated TraccarUser so the caller can
	// remember which Traccar user "owns" the tenant's session, and never
	// auto-pause that specific user later (see SetUserDisabled).
	Login(ctx context.Context, baseURL *url.URL, email, password string) (Session, TraccarUser, error)
	// CreateToken mints a long-lived API token for the session's own user, so
	// the scheduler can keep working after the login cookie expires. Traccar
	// may shorten the requested expiry, so the token is the return value and
	// the requested date is only a hint.
	CreateToken(ctx context.Context, baseURL *url.URL, session Session, expiresAt time.Time) (string, error)
	FetchUsers(ctx context.Context, baseURL *url.URL, session Session) ([]TraccarUser, error)
	FetchDevices(ctx context.Context, baseURL *url.URL, session Session) ([]TraccarDevice, error)
	// FetchDevicesForUser returns only the devices owned by traccarUserID.
	// Traccar devices carry no owner field of their own, so per-account
	// device counts require this per-user query instead of FetchDevices.
	FetchDevicesForUser(ctx context.Context, baseURL *url.URL, session Session, traccarUserID int64) ([]TraccarDevice, error)
	FetchServerInfo(ctx context.Context, baseURL *url.URL, session Session) (TraccarServerInfo, error)
	// SetUserDisabled enables or disables the Traccar user's own login,
	// used to pause access when a subscription goes overdue and restore it
	// on payment. It preserves every other field on the user record.
	SetUserDisabled(ctx context.Context, baseURL *url.URL, session Session, traccarUserID int64, disabled bool) error
	// DeleteUser removes the Traccar user outright. Deleting only the
	// billing account would be undone by the next sync, which recreates
	// an account for every user the server still returns.
	DeleteUser(ctx context.Context, baseURL *url.URL, session Session, traccarUserID int64) error
}

type TraccarUser struct {
	ID    int64
	Name  string
	Email string
}

type TraccarDevice struct {
	ID       int64
	Name     string
	UniqueID string
	Status   string
}

type TraccarServerInfo struct {
	Version string
}
