package api

import (
	"net/url"
	"testing"
)

func TestNormalizeTraccarBaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "root url gets /api appended", input: "https://gps.example.com", want: "https://gps.example.com/api"},
		{name: "root url with trailing slash", input: "https://gps.example.com/", want: "https://gps.example.com/api"},
		{name: "already has /api", input: "https://gps.example.com/api", want: "https://gps.example.com/api"},
		{name: "already has /api with trailing slash", input: "https://gps.example.com/api/", want: "https://gps.example.com/api"},
		{name: "custom sub-path before api", input: "https://example.com/traccar", want: "https://example.com/traccar/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.input)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			got := normalizeTraccarBaseURL(u)
			if got.String() != tt.want {
				t.Errorf("normalizeTraccarBaseURL(%q) = %q, want %q", tt.input, got.String(), tt.want)
			}
		})
	}
}
