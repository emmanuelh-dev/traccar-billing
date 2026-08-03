package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const (
	dueDateFormat   = "2006-01-02"
	defaultCurrency = "MXN"
)

// handleConfigureSubscription creates or edits a subscription's billing
// terms (amount, currency, period, next due date) from the dashboard form.
// Unlike handlePayAccount, this never records a payment or touches
// LastPaidAt: it is how an operator sets up billing for an account before
// any payment has happened.
func (s *Server) handleConfigureSubscription(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	accountID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid account id")
		return
	}

	account, err := s.repo.GetAccount(r.Context(), tenant.ID, accountID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "account not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get account for subscription config", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	unitPriceCents, err := parseAmountCents(orZero(r.FormValue("unit_price")))
	if err != nil {
		redirectPageError(w, r, "invalid unit price")
		return
	}
	amountCents, err := parseAmountCents(orZero(r.FormValue("amount")))
	if err != nil {
		redirectPageError(w, r, "invalid amount")
		return
	}
	flatFeeCents, err := parseAmountCents(orZero(r.FormValue("flat_fee")))
	if err != nil {
		redirectPageError(w, r, "invalid flat fee")
		return
	}
	if unitPriceCents == 0 && amountCents == 0 {
		redirectPageError(w, r, "set a unit price or a flat amount")
		return
	}
	periodDays, err := strconv.Atoi(r.FormValue("period_days"))
	if err != nil || periodDays <= 0 {
		redirectPageError(w, r, "invalid billing period")
		return
	}
	minDevices := optionalInt(r.FormValue("min_devices"), 0)
	graceDays := optionalInt(r.FormValue("grace_days"), 0)
	currency := currencyOrDefault(r.FormValue("currency"))

	mode := billing.ModeRolling
	anchorDay, dueDay := 1, 5
	var nextDueAt time.Time

	if r.FormValue("billing_mode") == string(billing.ModeCalendar) {
		mode = billing.ModeCalendar
		anchorDay = clampDayOfMonth(optionalInt(r.FormValue("anchor_day"), 1))
		dueDay = clampDayOfMonth(optionalInt(r.FormValue("due_day"), 5))
		nextDueAt = billing.NextCalendarDue(s.now(), dueDay)
	} else {
		nextDueAt, err = time.ParseInLocation(dueDateFormat, r.FormValue("next_due_at"), s.loc)
		if err != nil {
			redirectPageError(w, r, "invalid due date")
			return
		}
	}

	sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
	if err != nil && !errors.Is(err, billing.ErrNotFound) {
		s.logger.Error("api: get subscription for config", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	sub.AccountID = account.ID
	sub.BillingMode = mode
	sub.AnchorDay = anchorDay
	sub.DueDay = dueDay
	sub.AmountCents = amountCents
	sub.UnitPriceCents = unitPriceCents
	sub.FlatFeeCents = flatFeeCents
	sub.MinDevices = minDevices
	sub.GraceDays = graceDays
	sub.Currency = currency
	sub.PeriodDays = periodDays
	sub.NextDueAt = nextDueAt
	if billing.IsOverdue(sub, s.now()) {
		sub.Status = billing.StatusOverdue
	} else {
		sub.Status = billing.StatusActive
	}

	if _, err := s.repo.UpsertSubscription(r.Context(), sub); err != nil {
		s.logger.Error("api: upsert subscription config", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	s.syncTraccarAccess(r.Context(), tenant, account, sub.Status == billing.StatusOverdue)

	http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
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

func orZero(raw string) string {
	if raw == "" {
		return "0"
	}
	return raw
}

func clampDayOfMonth(day int) int {
	if day < 1 {
		return 1
	}
	if day > 31 {
		return 31
	}
	return day
}

func optionalInt(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func redirectPageError(w http.ResponseWriter, r *http.Request, message string) {
	target := redirectTarget(r, "/dashboard")
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	http.Redirect(w, r, target+sep+"error="+url.QueryEscape(message), http.StatusSeeOther)
}
