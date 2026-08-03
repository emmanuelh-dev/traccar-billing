package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const browserSessionTTL = 24 * time.Hour

type loginView struct {
	T       uiStrings
	Title   string
	BaseURL string
	Email   string
	Error   string
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	t := stringsFor(resolveLang(w, r))
	render(w, http.StatusOK, "login", loginView{T: t, Title: t.LoginTitle})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	t := stringsFor(resolveLang(w, r))

	if err := r.ParseForm(); err != nil {
		render(w, http.StatusBadRequest, "login", loginView{T: t, Title: t.LoginTitle, Error: t.InvalidForm})
		return
	}

	rawURL := r.FormValue("base_url")
	email := r.FormValue("email")
	password := r.FormValue("password")

	view := loginView{T: t, Title: t.LoginTitle, BaseURL: rawURL, Email: email}

	baseURL, err := url.Parse(rawURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		view.Error = t.InvalidURL
		render(w, http.StatusBadRequest, "login", view)
		return
	}
	baseURL = normalizeTraccarBaseURL(baseURL)

	session, loggedInUser, err := s.client.Login(r.Context(), baseURL, email, password)
	if err != nil {
		s.logger.Warn("api: traccar login failed", "base_url", baseURL.String(), "email", email, "error", err)
		view.Error = t.LoginFailed
		render(w, http.StatusUnauthorized, "login", view)
		return
	}

	// The tenant is this Traccar user on this server, never the server on its
	// own: two people who log into the same Traccar each get their own books.
	tenant, err := s.repo.GetTenantByOwner(r.Context(), baseURL.String(), loggedInUser.ID)
	switch {
	case errors.Is(err, billing.ErrNotFound):
		tenant, err = s.repo.CreateTenant(r.Context(), billing.Tenant{
			Name:               baseURL.Hostname(),
			BaseURL:            baseURL.String(),
			TraccarUserID:      loggedInUser.ID,
			OwnerEmail:         loggedInUser.Email,
			SessionCookie:      session.Cookie,
			SessionExpiresAt:   session.ExpiresAt,
			AdminTraccarUserID: loggedInUser.ID,
		})
		if err != nil {
			s.logger.Error("api: create tenant", "error", err)
			view.Error = t.InternalError
			render(w, http.StatusInternalServerError, "login", view)
			return
		}
	case err != nil:
		s.logger.Error("api: get tenant by owner", "error", err)
		view.Error = t.InternalError
		render(w, http.StatusInternalServerError, "login", view)
		return
	default:
		if err := s.repo.UpdateTenantSession(r.Context(), tenant.ID, session); err != nil {
			s.logger.Error("api: update tenant session", "error", err)
			view.Error = t.InternalError
			render(w, http.StatusInternalServerError, "login", view)
			return
		}
		// The owner is the account SetUserDisabled will never auto-pause, so
		// pausing can't lock out the person who is currently able to fix it.
		// This also fills in the email for tenants created before it existed.
		if err := s.repo.UpdateTenantOwner(r.Context(), tenant.ID, loggedInUser.ID, loggedInUser.Email); err != nil {
			s.logger.Error("api: update tenant owner", "error", err)
		}
		tenant.AdminTraccarUserID = loggedInUser.ID
		tenant.OwnerEmail = loggedInUser.Email
	}

	s.ensureAPIToken(r.Context(), &tenant, baseURL, session)

	cookieExpiresAt := session.ExpiresAt
	if maxExpiry := time.Now().Add(browserSessionTTL); maxExpiry.Before(cookieExpiresAt) {
		cookieExpiresAt = maxExpiry
	}
	if err := setSessionCookie(w, r, s.signer, tenant.ID, cookieExpiresAt); err != nil {
		s.logger.Error("api: set session cookie", "error", err)
		view.Error = t.InternalError
		render(w, http.StatusInternalServerError, "login", view)
		return
	}

	tenant.SessionCookie = session.Cookie
	tenant.SessionExpiresAt = session.ExpiresAt
	s.syncNow(r.Context(), tenant)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// apiTokenTTL is how long a minted Traccar token is asked to live. Traccar
// enforces its own ceiling and quietly shortens anything above it, so this is
// an upper bound rather than a promise.
const apiTokenTTL = 365 * 24 * time.Hour

// ensureAPIToken gives a tenant a Traccar token the first time it logs in
// without one, so the scheduler can still suspend overdue accounts during the
// weeks nobody opens the web UI.
//
// Failing to mint one is not a login failure: older Traccar versions have no
// token endpoint and some users lack the permission. The tenant simply keeps
// working off the cookie, which is what it did before tokens existed.
func (s *Server) ensureAPIToken(ctx context.Context, tenant *billing.Tenant, baseURL *url.URL, session billing.Session) {
	if tenant.APIToken != "" {
		return
	}

	token, err := s.client.CreateToken(ctx, baseURL, session, time.Now().Add(apiTokenTTL))
	if err != nil {
		s.logger.Warn("api: could not mint traccar token, falling back to the login cookie", "tenant_id", tenant.ID, "error", err)
		return
	}
	if err := s.repo.UpdateTenantAPIToken(ctx, tenant.ID, token); err != nil {
		s.logger.Error("api: store traccar token", "tenant_id", tenant.ID, "error", err)
		return
	}

	tenant.APIToken = token
	s.logger.Info("api: minted traccar token for tenant", "tenant_id", tenant.ID)
}

// syncNow pulls the tenant's accounts before the dashboard renders, so a
// freshly connected server is not blank until the scheduler's next tick.
// A failure only costs the operator that wait: the scheduler retries.
func (s *Server) syncNow(ctx context.Context, tenant billing.Tenant) {
	if s.syncer == nil {
		return
	}

	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loginSyncTimeout)
	defer cancel()

	if err := s.syncer.SyncTenant(syncCtx, tenant); err != nil {
		s.logger.Warn("api: initial sync after login failed", "tenant_id", tenant.ID, "error", err)
		return
	}
	s.logger.Info("api: initial sync after login", "tenant_id", tenant.ID)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// normalizeTraccarBaseURL appends /api when the operator enters just the
// server's root URL (the common case), so https://gps.example.com and
// https://gps.example.com/api both resolve to the same tenant.
func normalizeTraccarBaseURL(u *url.URL) *url.URL {
	normalized := *u
	path := strings.TrimRight(normalized.Path, "/")
	if !strings.HasSuffix(path, "/api") {
		path += "/api"
	}
	normalized.Path = path
	return &normalized
}
