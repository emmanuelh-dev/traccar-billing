package traccar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func TestClientLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("email") != "admin@example.com" || r.FormValue("password") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "abc123"})
		_ = json.NewEncoder(w).Encode(userDTO{ID: 7, Name: "Admin", Email: "admin@example.com"})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()

	session, user, err := client.Login(context.Background(), baseURL, "admin@example.com", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.Cookie != "JSESSIONID=abc123" {
		t.Errorf("Login() cookie = %q, want %q", session.Cookie, "JSESSIONID=abc123")
	}
	if session.ExpiresAt.IsZero() {
		t.Error("Login() ExpiresAt should not be zero")
	}
	if user.ID != 7 || user.Email != "admin@example.com" {
		t.Errorf("Login() user = %+v, want ID=7 Email=admin@example.com", user)
	}
}

func TestClientLoginBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()

	_, _, err := client.Login(context.Background(), baseURL, "admin@example.com", "wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Login() error = %v, want ErrUnauthorized", err)
	}
}

func TestClientFetchUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Cookie") != "JSESSIONID=abc123" {
			t.Fatalf("missing session cookie: %q", r.Header.Get("Cookie"))
		}
		_ = json.NewEncoder(w).Encode([]userDTO{{ID: 1, Name: "Ada", Email: "ada@example.com", Disabled: true}})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	users, err := client.FetchUsers(context.Background(), baseURL, session)
	if err != nil {
		t.Fatalf("FetchUsers() error = %v", err)
	}
	if len(users) != 1 || users[0].Email != "ada@example.com" {
		t.Errorf("FetchUsers() = %+v", users)
	}
	if !users[0].Disabled {
		t.Errorf("FetchUsers() Disabled = false, want true")
	}
}

func TestClientFetchDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" || r.URL.Query().Get("all") != "true" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]deviceDTO{{ID: 9, Name: "Truck 1", UniqueID: "TRK1", Status: "online"}})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	devices, err := client.FetchDevices(context.Background(), baseURL, session)
	if err != nil {
		t.Fatalf("FetchDevices() error = %v", err)
	}
	if len(devices) != 1 || devices[0].UniqueID != "TRK1" {
		t.Errorf("FetchDevices() = %+v", devices)
	}
}

func TestClientFetchDeviceProtocols(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/positions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]positionDTO{
			{DeviceID: 9, Protocol: "teltonika"},
			{DeviceID: 10, Protocol: "osmand"},
		})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	protocols, err := NewClient().FetchDeviceProtocols(
		context.Background(), baseURL, billing.Session{Cookie: "JSESSIONID=abc123"},
	)
	if err != nil {
		t.Fatalf("FetchDeviceProtocols() error = %v", err)
	}
	if protocols[9] != "teltonika" || protocols[10] != "osmand" {
		t.Errorf("FetchDeviceProtocols() = %+v", protocols)
	}
}

func TestClientFetchDevicesForUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" || r.URL.Query().Get("userId") != "7" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]deviceDTO{
			{ID: 1, Name: "Truck 1", UniqueID: "TRK1", Status: "online"},
			{ID: 2, Name: "Truck 2", UniqueID: "TRK2", Status: "offline"},
		})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	devices, err := client.FetchDevicesForUser(context.Background(), baseURL, session, 7)
	if err != nil {
		t.Fatalf("FetchDevicesForUser() error = %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("FetchDevicesForUser() = %+v, want 2 devices", devices)
	}
}

func TestClientFetchServerInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(serverDTO{Version: "6.5"})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	info, err := client.FetchServerInfo(context.Background(), baseURL, session)
	if err != nil {
		t.Fatalf("FetchServerInfo() error = %v", err)
	}
	if info.Version != "6.5" {
		t.Errorf("FetchServerInfo() = %+v", info)
	}
}

func TestClientFetchUsersUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=expired"}

	_, err := client.FetchUsers(context.Background(), baseURL, session)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("FetchUsers() error = %v, want ErrUnauthorized", err)
	}
}

func TestClientSetUserDisabledPreservesOtherFields(t *testing.T) {
	var putBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(1), "name": "Other User", "email": "other@example.com", "disabled": false},
				{"id": float64(7), "name": "Ada", "email": "ada@example.com", "disabled": false, "administrator": true, "phone": "555-1234"},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/users/7":
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(putBody)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	if err := client.SetUserDisabled(context.Background(), baseURL, session, 7, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v", err)
	}

	if putBody["disabled"] != true {
		t.Errorf("PUT body disabled = %v, want true", putBody["disabled"])
	}
	if putBody["administrator"] != true {
		t.Errorf("PUT body lost administrator field: %+v", putBody)
	}
	if putBody["phone"] != "555-1234" {
		t.Errorf("PUT body lost phone field: %+v", putBody)
	}
	if putBody["email"] != "ada@example.com" {
		t.Errorf("PUT body lost email field: %+v", putBody)
	}
}

func TestClientSetUserDisabledUserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": float64(1), "name": "Other"}})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api")
	client := NewClient()
	session := billing.Session{Cookie: "JSESSIONID=abc123"}

	if err := client.SetUserDisabled(context.Background(), baseURL, session, 999, true); err == nil {
		t.Error("SetUserDisabled() expected error for missing user, got nil")
	}
}
