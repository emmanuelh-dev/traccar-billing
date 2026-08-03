package api

import (
	"errors"
	"net/http"
	"sort"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type tenantPaymentRow struct {
	AccountName   string
	DateDisplay   string
	AmountDisplay string
	Note          string
	paidAtUnix    int64
}

type paymentsView struct {
	T      uiStrings
	Title  string
	Active string
	Tenant billing.Tenant
	Rows   []tenantPaymentRow
}

// handlePayments lists every payment ever recorded for the tenant, across
// all accounts, newest first. It composes existing per-account queries
// rather than adding a dedicated repository method, since this is a
// read-only aggregate view with no write path of its own.
func (s *Server) handlePayments(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for payments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view := paymentsView{T: t, Title: t.PaymentsPageTtl, Active: "payments", Tenant: tenant}

	for _, account := range accounts {
		sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
		if errors.Is(err, billing.ErrNotFound) {
			continue
		}
		if err != nil {
			s.logger.Error("api: get subscription for payments", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		payments, err := s.repo.ListPaymentsBySubscription(r.Context(), sub.ID)
		if err != nil {
			s.logger.Error("api: list payments for payments page", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, p := range payments {
			view.Rows = append(view.Rows, tenantPaymentRow{
				AccountName:   account.Name,
				DateDisplay:   p.PaidAt.Format(dueDateFormat),
				AmountDisplay: formatAmount(p.AmountCents, p.Currency),
				Note:          p.Note,
				paidAtUnix:    p.PaidAt.Unix(),
			})
		}
	}

	sort.Slice(view.Rows, func(i, j int) bool { return view.Rows[i].paidAtUnix > view.Rows[j].paidAtUnix })

	render(w, http.StatusOK, "payments", view)
}
