package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const dueDateFormat = "2006-01-02"

// handleConfigureSubscription creates or edits a subscription's billing
// terms (amount, currency, period, next due date) from the dashboard form.
// Unlike handlePayAccount, this never records a payment or touches
// LastPaidAt: it is how an operator sets up billing for an account before
// any payment has happened.
func (s *Server) handleConfigureSubscription(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	accountID, err := parseIDParam(r, "id")
	if err != nil {
		redirectDashboardError(w, r, "invalid account id")
		return
	}

	account, err := s.repo.GetAccount(r.Context(), tenant.ID, accountID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectDashboardError(w, r, "account not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get account for subscription config", "error", err)
		redirectDashboardError(w, r, "internal error")
		return
	}

	if err := r.ParseForm(); err != nil {
		redirectDashboardError(w, r, "invalid form")
		return
	}

	amountCents, err := parseAmountCents(r.FormValue("amount"))
	if err != nil {
		redirectDashboardError(w, r, "invalid amount")
		return
	}
	periodDays, err := strconv.Atoi(r.FormValue("period_days"))
	if err != nil || periodDays <= 0 {
		redirectDashboardError(w, r, "invalid billing period")
		return
	}
	nextDueAt, err := time.Parse(dueDateFormat, r.FormValue("next_due_at"))
	if err != nil {
		redirectDashboardError(w, r, "invalid due date")
		return
	}
	currency := r.FormValue("currency")
	if currency == "" {
		currency = "MXN"
	}

	sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
	if err != nil && !errors.Is(err, billing.ErrNotFound) {
		s.logger.Error("api: get subscription for config", "error", err)
		redirectDashboardError(w, r, "internal error")
		return
	}

	sub.AccountID = account.ID
	sub.AmountCents = amountCents
	sub.Currency = currency
	sub.PeriodDays = periodDays
	sub.NextDueAt = nextDueAt
	if billing.IsOverdue(sub, time.Now()) {
		sub.Status = billing.StatusOverdue
	} else {
		sub.Status = billing.StatusActive
	}

	if _, err := s.repo.UpsertSubscription(r.Context(), sub); err != nil {
		s.logger.Error("api: upsert subscription config", "error", err)
		redirectDashboardError(w, r, "internal error")
		return
	}

	s.syncTraccarAccess(r.Context(), tenant, account, sub.Status == billing.StatusOverdue)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// syncTraccarAccess mirrors a subscription's status onto the Traccar user's
// own disabled flag: paused when overdue, restored otherwise. Failures are
// logged, not surfaced, since the local billing state is already saved and
// the scheduler's checkOverdue retries the pause every tick regardless.
func (s *Server) syncTraccarAccess(ctx context.Context, tenant billing.Tenant, account billing.Account, disabled bool) {
	if disabled && account.TraccarUserID == tenant.AdminTraccarUserID {
		s.logger.Warn("api: refusing to pause the tenant's own admin user", "account_id", account.ID, "traccar_user_id", account.TraccarUserID)
		return
	}
	if !tenant.HasValidSession(time.Now()) {
		return
	}
	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		s.logger.Error("api: parse base url to sync traccar access", "error", err)
		return
	}
	session := billing.Session{Cookie: tenant.SessionCookie, ExpiresAt: tenant.SessionExpiresAt}

	if err := s.client.SetUserDisabled(ctx, baseURL, session, account.TraccarUserID, disabled); err != nil {
		s.logger.Error("api: sync traccar user access", "account_id", account.ID, "traccar_user_id", account.TraccarUserID, "disabled", disabled, "error", err)
		return
	}
	s.logger.Info("api: synced traccar user access", "account_id", account.ID, "traccar_user_id", account.TraccarUserID, "disabled", disabled)
}

func parseAmountCents(raw string) (int64, error) {
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil || amount < 0 {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	return int64(math.Round(amount * 100)), nil
}

func redirectDashboardError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/dashboard?error="+url.QueryEscape(message), http.StatusSeeOther)
}
