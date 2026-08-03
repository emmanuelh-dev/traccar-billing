package api

import (
	"context"
	"log/slog"
	"net/http"
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

type Server struct {
	repo   billing.Repository
	client billing.TraccarClient
	signer sessionSigner
	logger *slog.Logger
	loc    *time.Location
	syncer TenantSyncer
}

func (s *Server) SetSyncer(syncer TenantSyncer) {
	s.syncer = syncer
}

func NewServer(repo billing.Repository, client billing.TraccarClient, sessionSecret string, loc *time.Location, logger *slog.Logger) *Server {
	if loc == nil {
		loc = time.UTC
	}
	return &Server{
		repo:   repo,
		client: client,
		signer: newSessionSigner(sessionSecret),
		logger: logger,
		loc:    loc,
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
	r.Get("/payments", s.requireTenant(s.handlePayments))
	r.Get("/sellers", s.requireTenant(s.handleSellers))
	r.Post("/sellers", s.requireTenant(s.handleCreateSeller))
	r.Post("/sellers/{id}", s.requireTenant(s.handleUpdateSeller))
	r.Get("/settings", s.requireTenant(s.handleSettings))
	r.Post("/settings", s.requireTenant(s.handleSaveSettings))

	r.Get("/accounts", s.requireTenant(s.handleListAccounts))
	r.Get("/accounts/{id}", s.requireTenant(s.handleGetAccount))
	r.Post("/accounts/{id}/pay", s.requireTenant(s.handlePayAccount))
	r.Post("/accounts/{id}/subscription", s.requireTenant(s.handleConfigureSubscription))
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
