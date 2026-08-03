package billing

import (
	"testing"
	"time"
)

// TestHasValidSessionWithToken covers the reason the token exists: a tenant
// whose login cookie died weeks ago must still count as connected, because
// that is when the scheduler needs to go on suspending overdue accounts.
func TestHasValidSessionWithToken(t *testing.T) {
	now := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	expired := now.Add(-30 * 24 * time.Hour)

	tests := []struct {
		name   string
		tenant Tenant
		want   bool
	}{
		{
			name:   "token alone, no cookie at all",
			tenant: Tenant{APIToken: "tok"},
			want:   true,
		},
		{
			name:   "token survives a long-expired cookie",
			tenant: Tenant{APIToken: "tok", SessionCookie: "JSESSIONID=x", SessionExpiresAt: expired},
			want:   true,
		},
		{
			name:   "live cookie without a token",
			tenant: Tenant{SessionCookie: "JSESSIONID=x", SessionExpiresAt: now.Add(time.Hour)},
			want:   true,
		},
		{
			name:   "expired cookie without a token is what used to strand the scheduler",
			tenant: Tenant{SessionCookie: "JSESSIONID=x", SessionExpiresAt: expired},
			want:   false,
		},
		{
			name:   "nothing at all",
			tenant: Tenant{},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tenant.HasValidSession(now); got != tc.want {
				t.Errorf("HasValidSession() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTraccarSessionCarriesTheToken(t *testing.T) {
	tenant := Tenant{SessionCookie: "JSESSIONID=x", APIToken: "tok"}
	if got := tenant.TraccarSession(); got.Token != "tok" || got.Cookie != "JSESSIONID=x" {
		t.Errorf("TraccarSession() = %+v, want both credentials carried through", got)
	}
}
