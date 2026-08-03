package billing

import (
	"testing"
	"time"
)

func TestPeriodFor(t *testing.T) {
	locNY, err := time.LoadLocation("America/New_York")
	if err != nil {
		locNY = time.UTC
	}

	tests := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "regular 31-day month",
			now:       time.Date(2026, time.January, 15, 14, 30, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "february non-leap year",
			now:       time.Date(2025, time.February, 10, 8, 0, 0, 0, time.UTC),
			wantStart: time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "february leap year",
			now:       time.Date(2024, time.February, 29, 23, 59, 0, 0, time.UTC),
			wantStart: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "non-UTC timezone preserved",
			now:       time.Date(2026, time.August, 3, 1, 0, 0, 0, locNY),
			wantStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, locNY),
			wantEnd:   time.Date(2026, time.August, 31, 0, 0, 0, 0, locNY),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := PeriodFor(tt.now)
			if !gotStart.Equal(tt.wantStart) {
				t.Errorf("PeriodFor(%v) start = %v, want %v", tt.now, gotStart, tt.wantStart)
			}
			if !gotEnd.Equal(tt.wantEnd) {
				t.Errorf("PeriodFor(%v) end = %v, want %v", tt.now, gotEnd, tt.wantEnd)
			}
		})
	}
}

func TestBuildRemission(t *testing.T) {
	sub := Subscription{
		ID:             10,
		BillingMode:    ModeCalendar,
		UnitPriceCents: 500,
		FlatFeeCents:   1000,
		MinDevices:     2,
		Currency:       "MXN",
	}
	acc := Account{
		ID:          5,
		TenantID:    2,
		DeviceCount: 3,
	}
	now := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)

	rem := BuildRemission(sub, acc, now)

	if rem.TenantID != 2 {
		t.Errorf("TenantID = %d, want 2", rem.TenantID)
	}
	if rem.AccountID != 5 {
		t.Errorf("AccountID = %d, want 5", rem.AccountID)
	}
	if rem.SubscriptionID != 10 {
		t.Errorf("SubscriptionID = %d, want 10", rem.SubscriptionID)
	}
	if rem.DeviceCount != 3 {
		t.Errorf("DeviceCount = %d, want 3", rem.DeviceCount)
	}
	// 3 * 500 + 1000 = 2500
	if rem.AmountCents != 2500 {
		t.Errorf("AmountCents = %d, want 2500", rem.AmountCents)
	}
	if rem.Currency != "MXN" {
		t.Errorf("Currency = %s, want MXN", rem.Currency)
	}
	if rem.Status != RemissionPending {
		t.Errorf("Status = %s, want pending", rem.Status)
	}
	wantStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if !rem.IssuedAt.Equal(wantStart) {
		t.Errorf("IssuedAt = %v, want %v", rem.IssuedAt, wantStart)
	}

	// Test with device count below minimum
	acc.DeviceCount = 1
	remMin := BuildRemission(sub, acc, now)
	if remMin.DeviceCount != 2 {
		t.Errorf("DeviceCount = %d, want minimum 2", remMin.DeviceCount)
	}
	// 2 * 500 + 1000 = 2000
	if remMin.AmountCents != 2000 {
		t.Errorf("AmountCents = %d, want 2000", remMin.AmountCents)
	}
}

func TestShouldBill(t *testing.T) {
	tests := []struct {
		name string
		sub  Subscription
		want bool
	}{
		{
			name: "calendar active",
			sub:  Subscription{BillingMode: ModeCalendar, Status: StatusActive},
			want: true,
		},
		{
			name: "calendar overdue",
			sub:  Subscription{BillingMode: ModeCalendar, Status: StatusOverdue},
			want: true,
		},
		{
			name: "calendar suspended",
			sub:  Subscription{BillingMode: ModeCalendar, Status: StatusSuspended},
			want: true,
		},
		{
			name: "calendar canceled",
			sub:  Subscription{BillingMode: ModeCalendar, Status: StatusCanceled},
			want: false,
		},
		{
			name: "rolling active",
			sub:  Subscription{BillingMode: ModeRolling, Status: StatusActive},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldBill(tt.sub); got != tt.want {
				t.Errorf("ShouldBill() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemissionPending(t *testing.T) {
	r1 := Remission{Status: RemissionPending}
	r2 := Remission{Status: RemissionPaid}
	r3 := Remission{Status: RemissionCanceled}

	if !r1.Pending() {
		t.Errorf("r1.Pending() = false, want true")
	}
	if r2.Pending() {
		t.Errorf("r2.Pending() = true, want false")
	}
	if r3.Pending() {
		t.Errorf("r3.Pending() = true, want false")
	}
}
