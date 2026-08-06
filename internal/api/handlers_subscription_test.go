package api

import (
	"testing"
	"time"
)

func TestParseAmountCents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "whole number", input: "150", want: 15000},
		{name: "two decimals", input: "150.50", want: 15050},
		{name: "rounds to nearest cent", input: "10.005", want: 1001},
		{name: "zero", input: "0", want: 0},
		{name: "negative rejected", input: "-5", wantErr: true},
		{name: "garbage rejected", input: "abc", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAmountCents(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAmountCents(%q) expected error, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAmountCents(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseAmountCents(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestDaysUntilUsesCalendarDates(t *testing.T) {
	loc := time.FixedZone("test", -6*60*60)
	tests := []struct {
		name string
		now  time.Time
		due  time.Time
		want int
	}{
		{
			name: "tomorrow is one day even when less than 24 hours away",
			now:  time.Date(2026, 8, 4, 18, 0, 0, 0, loc),
			due:  time.Date(2026, 8, 5, 0, 0, 0, 0, loc),
			want: 1,
		},
		{
			name: "today is zero",
			now:  time.Date(2026, 8, 4, 18, 0, 0, 0, loc),
			due:  time.Date(2026, 8, 4, 23, 59, 0, 0, loc),
			want: 0,
		},
		{
			name: "yesterday is minus one",
			now:  time.Date(2026, 8, 4, 1, 0, 0, 0, loc),
			due:  time.Date(2026, 8, 3, 23, 0, 0, 0, loc),
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daysUntil(tt.due, tt.now); got != tt.want {
				t.Errorf("daysUntil() = %d, want %d", got, tt.want)
			}
		})
	}
}
