package traccar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// TestTokenIsPreferredOverCookie is the point of the whole token feature: once
// a tenant holds a token, every call must stop depending on the login cookie,
// which is the thing that expires and silently takes automatic suspension with
// it.
func TestTokenIsPreferredOverCookie(t *testing.T) {
	var gotAuth, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"version":"6.2"}`)); err != nil {
			t.Fatal(err)
		}
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}

	session := billing.Session{Cookie: "JSESSIONID=stale", Token: "tok-123"}
	if _, err := NewClient().FetchServerInfo(context.Background(), baseURL, session); err != nil {
		t.Fatalf("FetchServerInfo() error = %v", err)
	}

	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok-123")
	}
	if gotCookie != "" {
		t.Errorf("Cookie = %q, want it not sent at all once a token exists", gotCookie)
	}
}

func TestCookieIsUsedWhenThereIsNoToken(t *testing.T) {
	var gotAuth, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		if _, err := w.Write([]byte(`{"version":"6.2"}`)); err != nil {
			t.Fatal(err)
		}
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}

	session := billing.Session{Cookie: "JSESSIONID=live"}
	if _, err := NewClient().FetchServerInfo(context.Background(), baseURL, session); err != nil {
		t.Fatalf("FetchServerInfo() error = %v", err)
	}

	if gotCookie != "JSESSIONID=live" {
		t.Errorf("Cookie = %q, want the session cookie", gotCookie)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty without a token", gotAuth)
	}
}

func TestCreateToken(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "bare token", body: "abc.def.ghi", want: "abc.def.ghi"},
		{name: "quoted as a json string", body: `"abc.def.ghi"`, want: "abc.def.ghi"},
		{name: "trailing newline", body: "abc.def.ghi\n", want: "abc.def.ghi"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotExpiration string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/session/token" || r.Method != http.MethodPost {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				gotExpiration = r.FormValue("expiration")
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Fatal(err)
				}
			}))
			defer srv.Close()

			baseURL, err := url.Parse(srv.URL + "/api")
			if err != nil {
				t.Fatal(err)
			}

			expiresAt := time.Date(2027, time.January, 2, 3, 4, 5, 0, time.UTC)
			got, err := NewClient().CreateToken(context.Background(), baseURL, billing.Session{Cookie: "JSESSIONID=x"}, expiresAt)
			if err != nil {
				t.Fatalf("CreateToken() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("CreateToken() = %q, want %q", got, tc.want)
			}
			if gotExpiration != "2027-01-02T03:04:05Z" {
				t.Errorf("expiration = %q, want RFC3339 in UTC", gotExpiration)
			}
		})
	}
}

func TestCreateTokenRejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}

	// A blank token would be stored and then silently fail every later call,
	// which is exactly the invisible breakage this feature removes.
	if _, err := NewClient().CreateToken(context.Background(), baseURL, billing.Session{}, time.Now()); err == nil {
		t.Error("CreateToken() accepted an empty body, want an error")
	}
}
