package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type dashboardStats struct {
	Total   int
	Active  int
	Overdue int
}

type paymentRow struct {
	DateDisplay   string
	AmountDisplay string
	Note          string
}

type accountRow struct {
	Account         billing.Account
	Subscription    billing.Subscription
	HasSubscription bool
	StatusLabel     string
	AmountDisplay   string
	AmountValue     string
	DaysLeftLabel   string
	DefaultDueDate  string
	DefaultPeriod   int
	Payments        []paymentRow
}

type dashboardView struct {
	T      uiStrings
	Title  string
	Active string
	Error  string
	Tenant billing.Tenant
	Stats  dashboardStats
	Rows   []accountRow
}

const defaultPeriodDays = 30

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	now := time.Now()

	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view := dashboardView{T: t, Title: t.DashboardTitle, Active: "dashboard", Tenant: tenant, Error: r.URL.Query().Get("error")}
	for _, account := range accounts {
		row := accountRow{
			Account:        account,
			DefaultDueDate: now.AddDate(0, 0, defaultPeriodDays).Format(dueDateFormat),
			DefaultPeriod:  defaultPeriodDays,
		}
		view.Stats.Total++

		sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
		switch {
		case errors.Is(err, billing.ErrNotFound):
			// no subscription yet, row.HasSubscription stays false
		case err != nil:
			s.logger.Error("api: get subscription for dashboard", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			row.Subscription = sub
			row.HasSubscription = true
			row.StatusLabel = t.statusLabel(sub.Status)
			row.AmountDisplay = formatAmount(sub.AmountCents, sub.Currency)
			row.AmountValue = fmt.Sprintf("%.2f", float64(sub.AmountCents)/100)
			row.DaysLeftLabel = t.daysLeftLabel(daysUntil(sub.NextDueAt, now))
			row.DefaultDueDate = sub.NextDueAt.Format(dueDateFormat)
			row.DefaultPeriod = sub.PeriodDays

			payments, err := s.repo.ListPaymentsBySubscription(r.Context(), sub.ID)
			if err != nil {
				s.logger.Error("api: list payments for dashboard", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			for _, p := range payments {
				row.Payments = append(row.Payments, paymentRow{
					DateDisplay:   p.PaidAt.Format(dueDateFormat),
					AmountDisplay: formatAmount(p.AmountCents, p.Currency),
					Note:          p.Note,
				})
			}

			if sub.Status == billing.StatusOverdue {
				view.Stats.Overdue++
			} else if sub.Status == billing.StatusActive {
				view.Stats.Active++
			}
		}

		view.Rows = append(view.Rows, row)
	}

	render(w, http.StatusOK, "dashboard", view)
}

func formatAmount(cents int64, currency string) string {
	return fmt.Sprintf("%s %.2f", currency, float64(cents)/100)
}

func daysUntil(due, now time.Time) int {
	return int(due.Sub(now).Hours() / 24)
}
