package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// loginStubClient signs everyone in successfully and reports whoever the test
// says is behind the credentials, which is all the login handler needs to
// decide whose books to open.
type loginStubClient struct {
	stubTraccarClient
	users map[string]billing.TraccarUser
}

func (c *loginStubClient) Login(_ context.Context, _ *url.URL, email, _ string) (billing.Session, billing.TraccarUser, error) {
	return billing.Session{Cookie: "JSESSIONID=" + email, ExpiresAt: time.Now().Add(time.Hour)}, c.users[email], nil
}

func loginAs(t *testing.T, handler http.Handler, baseURL, email string) []*http.Cookie {
	t.Helper()

	form := url.Values{"base_url": {baseURL}, "email": {email}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login as %s: status = %d, want 303 (body: %s)", email, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func get(t *testing.T, handler http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestLoginIsolatesTenantsPerTraccarUser is the regression test for the bug
// where a tenant was a Traccar server: a second person logging into the same
// server landed in the first one's books and saw their accounts, payments and
// agenda.
func TestLoginIsolatesTenantsPerTraccarUser(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"first@example.com":  {ID: 11, Name: "First", Email: "first@example.com"},
		"second@example.com": {ID: 22, Name: "Second", Email: "second@example.com"},
	}}
	handler := srv.Router()
	ctx := context.Background()

	const server = "https://gps.example.com"

	firstCookies := loginAs(t, handler, server, "first@example.com")
	secondCookies := loginAs(t, handler, server, "second@example.com")

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("two users on one server produced %d tenants, want 2", len(tenants))
	}
	if tenants[0].TraccarUserID == tenants[1].TraccarUserID {
		t.Errorf("both tenants own Traccar user %d, want one each", tenants[0].TraccarUserID)
	}
	if tenants[1].OwnerEmail != "second@example.com" {
		t.Errorf("OwnerEmail = %q, want second@example.com", tenants[1].OwnerEmail)
	}

	// The first operator books a visit. It must stay invisible to the second.
	if _, err := repo.CreateAppointment(ctx, billing.Appointment{
		TenantID:    tenants[0].ID,
		ClientName:  "Cliente Privado",
		ScheduledOn: time.Now(),
		Status:      billing.AppointmentScheduled,
	}); err != nil {
		t.Fatalf("CreateAppointment() error = %v", err)
	}

	if body := get(t, handler, "/appointments", firstCookies).Body.String(); !strings.Contains(body, "Cliente Privado") {
		t.Error("the owner cannot see their own visit")
	}
	if body := get(t, handler, "/appointments", secondCookies).Body.String(); strings.Contains(body, "Cliente Privado") {
		t.Error("a different Traccar user of the same server can see someone else's visit")
	}
}

// TestLoginSameUserReusesTenant makes sure the split did not turn every login
// into a fresh empty set of books.
func TestLoginSameUserReusesTenant(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"owner@example.com": {ID: 7, Name: "Owner", Email: "owner@example.com"},
	}}
	handler := srv.Router()

	loginAs(t, handler, "https://gps.example.com", "owner@example.com")
	// The second login writes the URL the same way the first one did, even
	// though the operator typed it differently.
	loginAs(t, handler, "https://gps.example.com/api/", "owner@example.com")

	tenants, err := repo.ListTenants(context.Background())
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("logging in twice produced %d tenants, want 1", len(tenants))
	}
	if tenants[0].OwnerEmail != "owner@example.com" {
		t.Errorf("OwnerEmail = %q, want owner@example.com", tenants[0].OwnerEmail)
	}
}
