package connectivity

import (
	"context"
	"testing"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type stubProvider struct{ id string }

func (p stubProvider) ID() string { return p.id }
func (stubProvider) LookupDevice(context.Context, string) (billing.SIMInfo, error) {
	return billing.SIMInfo{}, nil
}
func (stubProvider) FetchUsage(context.Context, []string) ([]billing.UsageRecord, error) {
	return nil, nil
}

func TestRegistryResolvesDefaultAndTenantProvider(t *testing.T) {
	registry := NewRegistry()
	for _, provider := range []stubProvider{{id: "one"}, {id: "two"}} {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.SetDefault("one"); err != nil {
		t.Fatal(err)
	}
	if err := registry.AssignTenant(42, "two"); err != nil {
		t.Fatal(err)
	}

	if provider, _ := registry.ResolveProvider(billing.Tenant{ID: 7}); provider.ID() != "one" {
		t.Errorf("default provider = %q", provider.ID())
	}
	if provider, _ := registry.ResolveProvider(billing.Tenant{ID: 42}); provider.ID() != "two" {
		t.Errorf("tenant provider = %q", provider.ID())
	}
}

func TestRegistryBuildsProviderFromTenantCredential(t *testing.T) {
	registry := NewRegistry(func(encrypted string) (string, error) {
		if encrypted != "encrypted-token" {
			t.Fatalf("encrypted token = %q", encrypted)
		}
		return "plain-token", nil
	})
	if err := registry.RegisterFactory("one", func(token string) billing.ConnectivityProvider {
		if token != "plain-token" {
			t.Fatalf("factory token = %q", token)
		}
		return stubProvider{id: "one"}
	}); err != nil {
		t.Fatal(err)
	}
	provider, ok := registry.ResolveProvider(billing.Tenant{
		ID: 7, ConnectivityProvider: "one", ConnectivityToken: "encrypted-token",
	})
	if !ok || provider.ID() != "one" {
		t.Fatalf("ResolveProvider() = %v, %v", provider, ok)
	}
}
