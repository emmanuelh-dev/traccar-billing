package api

import "testing"

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
