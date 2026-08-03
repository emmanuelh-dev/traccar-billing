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
