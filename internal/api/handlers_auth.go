package api

import (
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

	tenant, err := s.repo.GetTenantByBaseURL(r.Context(), baseURL.String())
	switch {
	case errors.Is(err, billing.ErrNotFound):
		tenant, err = s.repo.CreateTenant(r.Context(), billing.Tenant{
			Name:               baseURL.Hostname(),
			BaseURL:            baseURL.String(),
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
		s.logger.Error("api: get tenant by base url", "error", err)
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
		// Whoever just logged in becomes the account SetUserDisabled will
		// never auto-pause, so pausing can't lock out the person who is
		// currently able to fix it.
		if err := s.repo.UpdateTenantAdmin(r.Context(), tenant.ID, loggedInUser.ID); err != nil {
			s.logger.Error("api: update tenant admin", "error", err)
		}
		tenant.AdminTraccarUserID = loggedInUser.ID
	}

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

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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
