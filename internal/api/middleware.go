package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type contextKey int

const tenantContextKey contextKey = iota

func tenantFromContext(ctx context.Context) billing.Tenant {
	t, _ := ctx.Value(tenantContextKey).(billing.Tenant)
	return t
}

// requireTenant enforces "no valid session means always ask for the
// password again": a missing browser cookie, an unknown/expired tenant, or
// a tenant whose stored Traccar session has expired all fall back to
// /login (or 401 for JSON callers), with no bypass.
func (s *Server) requireTenant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenantIDFromRequest(r, s.signer)
		if err != nil {
			s.denyUnauthenticated(w, r)
			return
		}

		tenant, err := s.repo.GetTenantByID(r.Context(), tenantID)
		if err != nil || !tenant.HasValidSession(time.Now()) {
			clearSessionCookie(w, r)
			s.denyUnauthenticated(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), tenantContextKey, tenant)
		s.warmDeviceData(tenant, stringsFor(resolveLang(w, r)))
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) denyUnauthenticated(w http.ResponseWriter, r *http.Request) {
	if isJSONPath(r.URL.Path) {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func isJSONPath(path string) bool {
	return strings.HasPrefix(path, "/accounts") ||
		path == "/devices/data" ||
		path == "/devices/list" ||
		strings.HasSuffix(path, "/sms/history") ||
		path == "/sim-history/data" ||
		path == "/sims/data" ||
		path == "/health"
}
