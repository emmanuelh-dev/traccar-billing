package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type remissionRow struct {
	Remission     billing.Remission
	AccountName   string
	AmountDisplay string
	PeriodDisplay string
	StatusLabel   string
	Pending       bool
}

var remissionStatusFilters = map[string]bool{
	"pending":  true,
	"paid":     true,
	"canceled": true,
	"all":      true,
}

type remissionsView struct {
	T      uiStrings
	Title  string
	Active string
	Error  string
	Tenant billing.Tenant
	Rows   []remissionRow

	Status        string
	PendingCount  int
	PendingAmount string
	Total         string

	// UnbilledCount is how many live accounts produced no remission because
	// nobody ever priced them. They are not an error, but they are invisible
	// otherwise, and an unpriced account is one nobody is charging.
	UnbilledCount int
	UnbilledNote  string

	SessionExpired bool
}

func (s *Server) handleRemissions(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	status := resolveChoice(w, r, "status", remissionStatusFilters, "pending")

	filter := billing.RemissionFilter{}
	if status != "all" {
		filter.Status = billing.RemissionStatus(status)
	}

	remissions, err := s.repo.ListRemissions(r.Context(), tenant.ID, filter)
	if err != nil {
		s.logger.Error("api: list remissions", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	unbilled, err := s.countUnbilledAccounts(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: count unbilled accounts", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows := make([]remissionRow, 0, len(remissions))
	var total, pendingCents int64
	pendingCount := 0
	currency := defaultCurrency

	for _, rem := range remissions {
		if rem.Currency != "" {
			currency = rem.Currency
		}
		total += rem.AmountCents
		if rem.Pending() {
			pendingCount++
			pendingCents += rem.AmountCents
		}

		rows = append(rows, remissionRow{
			Remission:     rem.Remission,
			AccountName:   rem.AccountName,
			AmountDisplay: formatAmount(rem.AmountCents, rem.Currency),
			PeriodDisplay: rem.PeriodStart.Format(dueDateFormat) + " – " + rem.PeriodEnd.Format(dueDateFormat),
			StatusLabel:   remissionStatusLabel(t, rem.Status),
			Pending:       rem.Pending(),
		})
	}

	render(w, http.StatusOK, "remissions", remissionsView{
		T:      t,
		Title:  t.RemissionsPageTtl,
		Active: "remissions",
		Error:  r.URL.Query().Get("error"),
		Tenant: tenant,
		Rows:   rows,

		Status:        status,
		PendingCount:  pendingCount,
		PendingAmount: formatAmount(pendingCents, currency),
		Total:         formatAmount(total, currency),

		UnbilledCount: unbilled,
		UnbilledNote:  unbilledNote(t, unbilled),

		SessionExpired: !tenant.HasValidSession(s.now()),
	})
}

// countUnbilledAccounts counts the live accounts that have no subscription, so
// the operator can see who the month-end run skipped instead of discovering it
// when the money does not arrive.
func (s *Server) countUnbilledAccounts(ctx context.Context, tenantID int64) (int, error) {
	accounts, err := s.repo.ListAccountsByTenant(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, acc := range accounts {
		if acc.Archived() || acc.Mirror() {
			continue
		}
		if _, err := s.repo.GetSubscriptionByAccountID(ctx, acc.ID); errors.Is(err, billing.ErrNotFound) {
			count++
			continue
		} else if err != nil {
			return 0, err
		}
	}
	return count, nil
}

func unbilledNote(t uiStrings, count int) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf(t.RemissionUnbilledFmt, count)
}

func remissionStatusLabel(t uiStrings, status billing.RemissionStatus) string {
	switch status {
	case billing.RemissionPaid:
		return t.RemissionPaidLabel
	case billing.RemissionCanceled:
		return t.RemissionCanceledLbl
	default:
		return t.RemissionPendingLbl
	}
}

// handleSettleRemission is the "marcar como pagada" action. It records the
// payment and advances the subscription in the same transaction as settling
// the remission, because a remission marked paid whose subscription never
// moved would be suspended by the scheduler on the next tick.
func (s *Server) handleSettleRemission(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	remissionID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid remission id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	rem, err := s.repo.GetRemission(r.Context(), tenant.ID, remissionID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "remission not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get remission to settle", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	if !rem.Pending() {
		redirectPageError(w, r, "that remission is not pending")
		return
	}

	paidAt := s.now()
	method := r.FormValue("method")
	reference := r.FormValue("reference")

	var account billing.Account
	err = s.repo.WithTx(r.Context(), func(tx billing.Repository) error {
		sub, txErr := tx.GetSubscription(r.Context(), rem.SubscriptionID)
		if txErr != nil {
			return txErr
		}
		if _, txErr = tx.UpsertSubscription(r.Context(), billing.ApplyPayment(sub, paidAt)); txErr != nil {
			return txErr
		}

		payment, txErr := tx.RecordPayment(r.Context(), billing.Payment{
			AccountID:      rem.AccountID,
			SubscriptionID: rem.SubscriptionID,
			AmountCents:    rem.AmountCents,
			DeviceCount:    rem.DeviceCount,
			Currency:       rem.Currency,
			Method:         method,
			Reference:      reference,
			PaidAt:         paidAt,
			Note:           rem.Note,
		})
		if txErr != nil {
			return txErr
		}

		if _, txErr = tx.SettleRemission(r.Context(), tenant.ID, rem.ID, payment.ID, paidAt); txErr != nil {
			return txErr
		}

		account, txErr = tx.GetAccount(r.Context(), tenant.ID, rem.AccountID)
		return txErr
	})
	if err != nil {
		s.logger.Error("api: settle remission", "remission_id", rem.ID, "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	// Paying clears the suspension, the same as any other payment does.
	s.syncTraccarAccess(r.Context(), tenant, account, false)

	http.Redirect(w, r, redirectTarget(r, "/remissions"), http.StatusSeeOther)
}

func (s *Server) handleCancelRemission(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	remissionID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid remission id")
		return
	}

	err = s.repo.CancelRemission(r.Context(), tenant.ID, remissionID, time.Now().UTC())
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "remission not found")
		return
	}
	if err != nil {
		s.logger.Error("api: cancel remission", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, redirectTarget(r, "/remissions"), http.StatusSeeOther)
}
