package billing

import "time"

type Tenant struct {
	ID                 int64
	Name               string
	BaseURL            string
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
