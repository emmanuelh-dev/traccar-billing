package billing

import (
	"testing"
	"time"
)

func TestIsOverdue(t *testing.T) {
	now := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		sub  Subscription
		want bool
	}{
		{
			name: "due date in the past",
			sub:  Subscription{Status: StatusActive, NextDueAt: now.AddDate(0, 0, -1)},
			want: true,
		},
		{
			name: "due date in the future",
			sub:  Subscription{Status: StatusActive, NextDueAt: now.AddDate(0, 0, 1)},
			want: false,
		},
		{
			name: "due date exactly now is not yet overdue",
			sub:  Subscription{Status: StatusActive, NextDueAt: now},
			want: false,
		},
		{
			name: "canceled subscriptions are never overdue",
			sub:  Subscription{Status: StatusCanceled, NextDueAt: now.AddDate(0, 0, -30)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOverdue(tt.sub, now); got != tt.want {
				t.Errorf("IsOverdue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextDueDate(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodDays int
		want       time.Time
	}{
		{name: "monthly-ish 30 days", periodDays: 30, want: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)},
		{name: "annual 365 days", periodDays: 365, want: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "zero days", periodDays: 0, want: from},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextDueDate(from, tt.periodDays); !got.Equal(tt.want) {
				t.Errorf("NextDueDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPayment(t *testing.T) {
	sub := Subscription{
		Status:     StatusOverdue,
		PeriodDays: 30,
		NextDueAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	paidAt := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

	got := ApplyPayment(sub, paidAt)

	if got.Status != StatusActive {
		t.Errorf("ApplyPayment() status = %v, want %v", got.Status, StatusActive)
	}
	if !got.LastPaidAt.Equal(paidAt) {
		t.Errorf("ApplyPayment() LastPaidAt = %v, want %v", got.LastPaidAt, paidAt)
	}
	wantNextDue := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if !got.NextDueAt.Equal(wantNextDue) {
		t.Errorf("ApplyPayment() NextDueAt = %v, want %v", got.NextDueAt, wantNextDue)
	}
}

func TestChargeCents(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subscription
		devices int
		want    int64
	}{
		{
			name:    "flat amount when no unit price",
			sub:     Subscription{AmountCents: 20000},
			devices: 11,
			want:    20000,
		},
		{
			name:    "per device",
			sub:     Subscription{UnitPriceCents: 20000},
			devices: 11,
			want:    220000,
		},
		{
			name:    "per device with base fee",
			sub:     Subscription{UnitPriceCents: 20000, FlatFeeCents: 5000},
			devices: 3,
			want:    65000,
		},
		{
			name:    "minimum billable applies",
			sub:     Subscription{UnitPriceCents: 20000, MinDevices: 5},
			devices: 2,
			want:    100000,
		},
		{
			name:    "minimum ignored above it",
			sub:     Subscription{UnitPriceCents: 20000, MinDevices: 5},
			devices: 7,
			want:    140000,
		},
		{
			name:    "zero devices",
			sub:     Subscription{UnitPriceCents: 20000},
			devices: 0,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChargeCents(tt.sub, tt.devices); got != tt.want {
				t.Errorf("ChargeCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsOverdueWithGrace(t *testing.T) {
	due := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sub := Subscription{Status: StatusActive, NextDueAt: due, GraceDays: 5}

	if IsOverdue(sub, due.AddDate(0, 0, 3)) {
		t.Error("IsOverdue() inside the grace period = true, want false")
	}
	if IsOverdue(sub, due.AddDate(0, 0, 5)) {
		t.Error("IsOverdue() on the last grace day = true, want false")
	}
	if !IsOverdue(sub, due.AddDate(0, 0, 6)) {
		t.Error("IsOverdue() past the grace period = false, want true")
	}
}
