package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolvePeriod(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	s := &Server{loc: mexico}

	tests := []struct {
		name       string
		query      string
		wantPeriod string
		wantFrom   string
		wantTo     string
	}{
		{name: "unknown period means everything", query: "?period=garbage", wantPeriod: "all"},
		{name: "all is explicit", query: "?period=all", wantPeriod: "all"},
		{
			name:       "explicit range is inclusive of the end day",
			query:      "?period=range&from=2026-07-01&to=2026-07-31",
			wantPeriod: "range",
			wantFrom:   "2026-07-01",
			wantTo:     "2026-08-01",
		},
		{
			name:       "range accepts an open start",
			query:      "?period=range&to=2026-07-31",
			wantPeriod: "range",
			wantTo:     "2026-08-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/payments"+tt.query, nil)
			period, filter := s.resolvePeriod(r)

			if period != tt.wantPeriod {
				t.Errorf("period = %q, want %q", period, tt.wantPeriod)
			}
			gotFrom, gotTo := "", ""
			if !filter.From.IsZero() {
				gotFrom = filter.From.Format(dueDateFormat)
			}
			if !filter.To.IsZero() {
				gotTo = filter.To.Format(dueDateFormat)
			}
			if gotFrom != tt.wantFrom {
				t.Errorf("from = %q, want %q", gotFrom, tt.wantFrom)
			}
			if gotTo != tt.wantTo {
				t.Errorf("to = %q, want %q", gotTo, tt.wantTo)
			}
		})
	}
}

func TestResolvePeriodMonthShortcuts(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	s := &Server{loc: mexico}
	now := s.now()

	current, currentFilter := s.resolvePeriod(httptest.NewRequest("GET", "/payments?period=current", nil))
	if current != "current" {
		t.Fatalf("period = %q, want current", current)
	}
	if currentFilter.From.Day() != 1 || currentFilter.From.Month() != now.Month() {
		t.Errorf("current from = %v, want the 1st of %v", currentFilter.From, now.Month())
	}
	if !currentFilter.To.Equal(currentFilter.From.AddDate(0, 1, 0)) {
		t.Errorf("current to = %v, want one month after %v", currentFilter.To, currentFilter.From)
	}

	defaultPeriod, defaultFilter := s.resolvePeriod(httptest.NewRequest("GET", "/payments", nil))
	if defaultPeriod != "current" {
		t.Errorf("period with no query = %q, want current", defaultPeriod)
	}
	if !defaultFilter.From.Equal(currentFilter.From) || !defaultFilter.To.Equal(currentFilter.To) {
		t.Errorf("default filter = %v..%v, want the current month %v..%v", defaultFilter.From, defaultFilter.To, currentFilter.From, currentFilter.To)
	}

	_, prevFilter := s.resolvePeriod(httptest.NewRequest("GET", "/payments?period=previous", nil))
	if !prevFilter.To.Equal(currentFilter.From) {
		t.Errorf("previous ends at %v, want the start of the current month %v", prevFilter.To, currentFilter.From)
	}
	if !prevFilter.From.Equal(currentFilter.From.AddDate(0, -1, 0)) {
		t.Errorf("previous starts at %v, want one month before %v", prevFilter.From, currentFilter.From)
	}
}
