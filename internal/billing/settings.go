package billing

import "time"

// Settings holds a tenant's billing defaults: the values the dashboard
// pre-fills into a new subscription instead of hardcoding them, so an
// operator configures a price once instead of on every account.
type Settings struct {
	TenantID       int64
	BillingMode    BillingMode
	AnchorDay      int
	DueDay         int
	PeriodDays     int
	GraceDays      int
	Currency       string
	UnitPriceCents int64
	FlatFeeCents   int64
	MinDevices     int
	// HideMirrorAccounts keeps Traccar's temporary device-share users out
	// of the dashboard. See Account.Mirror.
	HideMirrorAccounts bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// DefaultSettings is what a tenant gets before it ever opens the settings
// panel, and the fallback when a row predates this feature.
func DefaultSettings(tenantID int64) Settings {
	return Settings{
		TenantID:           tenantID,
		BillingMode:        ModeRolling,
		AnchorDay:          1,
		DueDay:             5,
		PeriodDays:         30,
		GraceDays:          5,
		Currency:           "MXN",
		HideMirrorAccounts: true,
	}
}

// Normalized clamps stored values back into the ranges the rest of the
// package assumes, so a hand-edited row cannot produce a zero-day period
// or a due day of 40.
func (s Settings) Normalized() Settings {
	if s.BillingMode != ModeCalendar {
		s.BillingMode = ModeRolling
	}
	if s.PeriodDays <= 0 {
		s.PeriodDays = 30
	}
	if s.GraceDays < 0 {
		s.GraceDays = 0
	}
	if s.MinDevices < 0 {
		s.MinDevices = 0
	}
	if s.Currency == "" {
		s.Currency = "MXN"
	}
	s.AnchorDay = clampDay(s.AnchorDay, 1)
	s.DueDay = clampDay(s.DueDay, 5)
	return s
}

func clampDay(day, fallback int) int {
	if day < 1 || day > 31 {
		return fallback
	}
	return day
}
