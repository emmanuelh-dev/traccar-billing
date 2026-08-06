package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const loginSyncTimeout = 25 * time.Second

// TenantSyncer is implemented by *scheduler.Scheduler. It lets the login
// handler pull accounts immediately instead of leaving the dashboard empty
// until the next scheduler tick.
type TenantSyncer interface {
	SyncTenant(ctx context.Context, t billing.Tenant) error
}

type CredentialCodec interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

type Server struct {
	repo            billing.Repository
	client          billing.TraccarClient
	connectivity    billing.ConnectivityProviderResolver
	signer          sessionSigner
	logger          *slog.Logger
	loc             *time.Location
	syncer          TenantSyncer
	credentialCodec CredentialCodec

	deviceDataMu    sync.Mutex
	deviceDataCache map[string]deviceDataCacheEntry
	deviceDataLoads map[string]chan struct{}
}

func (s *Server) SetSyncer(syncer TenantSyncer) {
	s.syncer = syncer
}

func (s *Server) SetConnectivityProviderResolver(resolver billing.ConnectivityProviderResolver) {
	s.connectivity = resolver
}

func (s *Server) SetCredentialCodec(codec CredentialCodec) {
	s.credentialCodec = codec
}

func NewServer(repo billing.Repository, client billing.TraccarClient, sessionSecret string, loc *time.Location, logger *slog.Logger) *Server {
	if loc == nil {
		loc = time.UTC
	}
	return &Server{
		repo:            repo,
		client:          client,
		signer:          newSessionSigner(sessionSecret),
		logger:          logger,
		loc:             loc,
		deviceDataCache: make(map[string]deviceDataCacheEntry),
		deviceDataLoads: make(map[string]chan struct{}),
	}
}

func (s *Server) now() time.Time {
	return time.Now().In(s.loc)
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", s.handleHealth)

	r.Get("/login", s.handleLoginPage)
	r.Post("/login", s.handleLoginSubmit)
	r.Post("/logout", s.handleLogout)

	r.Get("/dashboard", s.requireTenant(s.handleDashboard))
	r.Get("/devices", s.requireTenant(s.handleDevices))
	r.Get("/devices/list", s.requireTenant(s.handleDevicesList))
	r.Get("/devices/data", s.requireTenant(s.handleDevicesData))
	r.Get("/devices/{imei}/sms/history", s.requireTenant(s.handleDeviceSMSHistory))
	r.Post("/devices/{imei}/sms", s.requireTenant(s.handleSendDeviceSMS))
	r.Get("/sim-history", s.requireTenant(s.handleSIMHistory))
	r.Get("/sim-history/data", s.requireTenant(s.handleSIMHistoryData))
	r.Get("/sims", s.requireTenant(s.handleSIMs))
	r.Get("/sims/data", s.requireTenant(s.handleSIMsData))
	r.Post("/sims/status", s.requireTenant(s.handleSIMStatus))
	r.Get("/payments", s.requireTenant(s.handlePayments))
	r.Get("/expenses", s.requireTenant(s.handleExpenses))
	r.Post("/expenses", s.requireTenant(s.handleCreateExpense))
	r.Post("/expenses/{id}", s.requireTenant(s.handleUpdateExpense))
	r.Post("/expenses/{id}/delete", s.requireTenant(s.handleDeleteExpense))
	r.Get("/appointments", s.requireTenant(s.handleAppointments))
	r.Post("/appointments", s.requireTenant(s.handleCreateAppointment))
	r.Post("/appointments/{id}", s.requireTenant(s.handleUpdateAppointment))
	r.Post("/appointments/{id}/status", s.requireTenant(s.handleAppointmentStatus))
	r.Post("/appointments/{id}/delete", s.requireTenant(s.handleDeleteAppointment))
	r.Get("/sellers", s.requireTenant(s.handleSellers))
	r.Post("/sellers", s.requireTenant(s.handleCreateSeller))
	r.Post("/sellers/{id}", s.requireTenant(s.handleUpdateSeller))
	r.Get("/concepts", s.requireTenant(s.handleConcepts))
	r.Post("/concepts", s.requireTenant(s.handleCreateConcept))
	r.Post("/concepts/{id}", s.requireTenant(s.handleUpdateConcept))
	r.Post("/concepts/{id}/delete", s.requireTenant(s.handleDeleteConcept))
	r.Get("/remissions", s.requireTenant(s.handleRemissions))
	r.Post("/remissions/{id}/pay", s.requireTenant(s.handleSettleRemission))
	r.Post("/remissions/{id}/cancel", s.requireTenant(s.handleCancelRemission))
	r.Get("/settings", s.requireTenant(s.handleSettings))
	r.Post("/settings", s.requireTenant(s.handleSaveSettings))
	r.Post("/settings/token", s.requireTenant(s.handleSaveAPIToken))
	r.Post("/settings/token/generate", s.requireTenant(s.handleGenerateAPIToken))
	r.Post("/settings/token/delete", s.requireTenant(s.handleDeleteAPIToken))
	r.Post("/settings/connectivity", s.requireTenant(s.handleSaveConnectivity))
	r.Post("/settings/connectivity/delete", s.requireTenant(s.handleDeleteConnectivity))

	r.Get("/accounts", s.requireTenant(s.handleListAccounts))
	r.Get("/accounts/{id}", s.requireTenant(s.handleGetAccount))
	r.Post("/accounts/{id}/pay", s.requireTenant(s.handlePayAccount))
	r.Post("/accounts/{id}/subscription", s.requireTenant(s.handleConfigureSubscription))
	r.Post("/accounts/{id}/subscription/reset", s.requireTenant(s.handleResetSubscriptionPeriod))
	r.Post("/accounts/{id}/seller", s.requireTenant(s.handleAssignSeller))
	r.Post("/accounts/{id}/delete", s.requireTenant(s.handleDeleteAccount))

	r.Post("/payments/{id}", s.requireTenant(s.handleEditPayment))
	r.Post("/payments/{id}/void", s.requireTenant(s.handleVoidPayment))
	r.Post("/payments/{id}/delete", s.requireTenant(s.handleDeletePayment))

	r.Handle("/static/*", staticHandler())

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	return r
}
