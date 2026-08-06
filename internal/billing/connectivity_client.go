package billing

import (
	"context"
	"errors"
	"time"
)

// ErrProviderUnauthorized lets callers tell a rejected credential apart from a
// provider that answered but could not be read. Reporting every failure as a
// bad token sends operators chasing the wrong thing.
var ErrProviderUnauthorized = errors.New("connectivity: provider rejected the credential")

// ConnectivityProvider is the provider-neutral contract used by the UI.
// Implementations translate their own API formats into these common types.
type ConnectivityProvider interface {
	ID() string
	LookupDevice(ctx context.Context, imei string) (SIMInfo, error)
	FetchUsage(ctx context.Context, iccids []string) ([]UsageRecord, error)
}

// HistoricalUsageProvider is an optional capability for providers that can
// report usage over an explicit range. It allows lifetime balances without
// forcing every provider to support the same reporting windows.
type HistoricalUsageProvider interface {
	FetchUsageRange(ctx context.Context, iccids []string, start, end time.Time) ([]UsageRecord, error)
}

type SIMInventoryProvider interface {
	ListSIMs(ctx context.Context) ([]SIMInfo, error)
}

// SIMStatusProvider is an optional provider capability for providers that
// support changing a SIM's activation status.
type SIMStatusProvider interface {
	ChangeSIMStatus(ctx context.Context, iccids []string, status string) error
}

type ConnectivityProviderConfigurer interface {
	ProviderForCredential(providerID, token string) (ConnectivityProvider, bool)
}

// ConnectivityProviderResolver selects a provider explicitly for a tenant.
// Selection must never be guessed from an ICCID prefix.
type ConnectivityProviderResolver interface {
	ResolveProvider(tenant Tenant) (ConnectivityProvider, bool)
}

// SMSProvider is an optional provider capability. A connectivity provider can
// implement data usage without implementing SMS management.
type SMSProvider interface {
	SendSMS(ctx context.Context, iccid, text string) (SMSReceipt, error)
	FetchSMSHistory(ctx context.Context, iccid string, limit int) ([]SMSMessage, error)
}

type SIMInfo struct {
	IMEI          string
	ICCID         string
	Label         string
	Description   string
	Status        string
	ServicePlan   string
	ActivatedAt   string
	AllowedData   string
	RemainingData string
}

type SMSReceipt struct {
	ID     string
	Detail string
}

type SMSMessage struct {
	ID             string `json:"id"`
	DateSubmitted  string `json:"dateSubmitted"`
	Content        string `json:"content"`
	ICCID          string `json:"iccid"`
	DeliveryStatus string `json:"deliveryStatus"`
}

type UsageRecord struct {
	ICCID      string
	TotalUsage string
	StartDate  string
	EndDate    string
}
