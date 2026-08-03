package api

import (
	"context"
	"testing"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// TestLoginMintsAPIToken covers the "verifica y lo crea" half of the feature:
// an operator should never have to know a token exists for the scheduler to
// keep working after their cookie expires.
func TestLoginMintsAPIToken(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"owner@example.com": {ID: 7, Name: "Owner", Email: "owner@example.com"},
	}}
	handler := srv.Router()

	loginAs(t, handler, "https://gps.example.com", "owner@example.com")

	tenants, err := repo.ListTenants(context.Background())
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("len(tenants) = %d, want 1", len(tenants))
	}
	if tenants[0].APIToken != "stub-token" {
		t.Errorf("APIToken = %q, want the token minted at login", tenants[0].APIToken)
	}
}

// TestLoginKeepsExistingAPIToken makes sure signing in again does not churn a
// working token, which would leave a trail of live tokens on the Traccar side.
func TestLoginKeepsExistingAPIToken(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"owner@example.com": {ID: 7, Name: "Owner", Email: "owner@example.com"},
	}}
	handler := srv.Router()
	ctx := context.Background()

	loginAs(t, handler, "https://gps.example.com", "owner@example.com")

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if err := repo.UpdateTenantAPIToken(ctx, tenants[0].ID, "token-from-before"); err != nil {
		t.Fatalf("UpdateTenantAPIToken() error = %v", err)
	}

	loginAs(t, handler, "https://gps.example.com", "owner@example.com")

	tenants, err = repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	if tenants[0].APIToken != "token-from-before" {
		t.Errorf("APIToken = %q, want the existing token left alone", tenants[0].APIToken)
	}
}
