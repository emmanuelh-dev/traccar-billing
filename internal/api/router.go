package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type Server struct {
	repo   billing.Repository
	client billing.TraccarClient
	signer sessionSigner
	logger *slog.Logger
	loc    *time.Location
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

	r.Get("/accounts", s.requireTenant(s.handleListAccounts))
	r.Get("/accounts/{id}", s.requireTenant(s.handleGetAccount))
	r.Post("/accounts/{id}/pay", s.requireTenant(s.handlePayAccount))
	r.Post("/accounts/{id}/subscription", s.requireTenant(s.handleConfigureSubscription))

	r.Post("/payments/{id}", s.requireTenant(s.handleEditPayment))
	r.Post("/payments/{id}/void", s.requireTenant(s.handleVoidPayment))

	r.Handle("/static/*", staticHandler())

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	return r
}
