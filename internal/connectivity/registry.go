package connectivity

import (
	"fmt"
	"sync"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// Registry keeps provider selection out of HTTP handlers. Today the default
// comes from deployment configuration; tenant assignments are supported so a
// persisted settings field can be added later without changing /devices.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]billing.ConnectivityProvider
	factories map[string]func(string) billing.ConnectivityProvider
	decrypt   func(string) (string, error)
	defaultID string
	tenantIDs map[int64]string
}

func NewRegistry(decryptors ...func(string) (string, error)) *Registry {
	registry := &Registry{
		providers: make(map[string]billing.ConnectivityProvider),
		factories: make(map[string]func(string) billing.ConnectivityProvider),
		tenantIDs: make(map[int64]string),
	}
	if len(decryptors) > 0 {
		registry.decrypt = decryptors[0]
	}
	return registry
}

func (r *Registry) Register(provider billing.ConnectivityProvider) error {
	if provider == nil || provider.ID() == "" {
		return fmt.Errorf("connectivity: provider id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[provider.ID()]; exists {
		return fmt.Errorf("connectivity: provider %q is already registered", provider.ID())
	}
	r.providers[provider.ID()] = provider
	return nil
}

func (r *Registry) RegisterFactory(providerID string, factory func(string) billing.ConnectivityProvider) error {
	if providerID == "" || factory == nil {
		return fmt.Errorf("connectivity: provider id and factory are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[providerID]; exists {
		return fmt.Errorf("connectivity: provider factory %q is already registered", providerID)
	}
	r.factories[providerID] = factory
	return nil
}

func (r *Registry) ProviderForCredential(providerID, token string) (billing.ConnectivityProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory := r.factories[providerID]
	if factory == nil || token == "" {
		return nil, false
	}
	return factory(token), true
}

func (r *Registry) SetDefault(providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[providerID]; !exists {
		return fmt.Errorf("connectivity: provider %q is not registered", providerID)
	}
	r.defaultID = providerID
	return nil
}

func (r *Registry) AssignTenant(tenantID int64, providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[providerID]; !exists {
		return fmt.Errorf("connectivity: provider %q is not registered", providerID)
	}
	r.tenantIDs[tenantID] = providerID
	return nil
}

func (r *Registry) ResolveProvider(tenant billing.Tenant) (billing.ConnectivityProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if tenant.ConnectivityProvider != "" && tenant.ConnectivityToken != "" && r.decrypt != nil {
		factory := r.factories[tenant.ConnectivityProvider]
		token, err := r.decrypt(tenant.ConnectivityToken)
		if factory == nil || err != nil || token == "" {
			return nil, false
		}
		return factory(token), true
	}

	providerID := r.tenantIDs[tenant.ID]
	if providerID == "" {
		providerID = r.defaultID
	}
	provider, ok := r.providers[providerID]
	return provider, ok
}
