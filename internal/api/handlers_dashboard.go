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

type accountRow struct {
	Account         billing.Account
	Subscription    billing.Subscription
	HasSubscription bool
	StatusLabel     string
	AmountDisplay   string
	AmountValue     string
	UnitPriceValue  string
	FlatFeeValue    string
	ChargeValue     string
	MinDevices      int
	GraceDays       int
	DaysLeftLabel   string
	DefaultDueDate  string
	DefaultPeriod   int
	Currency        string
	SellerID        int64
	SellerName      string
	BillingMode     string
	AnchorDay       int
	DueDay          int
	PaymentCount    int
}

type dashboardView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Stats    dashboardStats
	Rows     []accountRow
	Accounts []chargeAccount
	Sellers  []sellerOption
	Today    string
	Redirect string

	SessionExpired bool
}

const defaultPeriodDays = 30

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	now := s.now()

	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sellerOpts, sellerNames, err := s.sellerOptions(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list sellers for dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view := dashboardView{
		T:        t,
		Title:    t.DashboardTitle,
		Active:   "dashboard",
		Tenant:   tenant,
		Error:    r.URL.Query().Get("error"),
		Sellers:  sellerOpts,
		Today:    now.Format(dueDateFormat),
		Redirect: "/dashboard",

		SessionExpired: !tenant.HasValidSession(time.Now()),
	}

	for _, account := range accounts {
		row := accountRow{
			Account:        account,
			DefaultDueDate: now.AddDate(0, 0, defaultPeriodDays).Format(dueDateFormat),
			DefaultPeriod:  defaultPeriodDays,
			Currency:       defaultCurrency,
			UnitPriceValue: "0.00",
			FlatFeeValue:   "0.00",
			AmountValue:    "0.00",
			ChargeValue:    "0.00",
			BillingMode:    string(billing.ModeRolling),
			AnchorDay:      1,
			DueDay:         5,
			SellerID:       account.SellerID,
			SellerName:     sellerNames[account.SellerID],
		}
		view.Stats.Total++

		sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
		switch {
		case errors.Is(err, billing.ErrNotFound):
		case err != nil:
			s.logger.Error("api: get subscription for dashboard", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			chargeCents := billing.ChargeCents(sub, account.DeviceCount)

			row.Subscription = sub
			row.HasSubscription = true
			row.StatusLabel = t.statusLabel(sub.Status)
			row.AmountDisplay = formatAmount(chargeCents, sub.Currency)
			row.AmountValue = centsValue(sub.AmountCents)
			row.UnitPriceValue = centsValue(sub.UnitPriceCents)
			row.FlatFeeValue = centsValue(sub.FlatFeeCents)
			row.ChargeValue = centsValue(chargeCents)
			row.MinDevices = sub.MinDevices
			row.GraceDays = sub.GraceDays
			row.Currency = sub.Currency
			row.DaysLeftLabel = t.daysLeftLabel(daysUntil(sub.NextDueAt, now))
			row.DefaultDueDate = sub.NextDueAt.In(s.loc).Format(dueDateFormat)
			row.DefaultPeriod = sub.PeriodDays
			if sub.BillingMode != "" {
				row.BillingMode = string(sub.BillingMode)
			}
			if sub.AnchorDay > 0 {
				row.AnchorDay = sub.AnchorDay
			}
			if sub.DueDay > 0 {
				row.DueDay = sub.DueDay
			}

			payments, err := s.repo.ListPaymentsBySubscription(r.Context(), sub.ID)
			if err != nil {
				s.logger.Error("api: list payments for dashboard", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			row.PaymentCount = len(payments)

			if sub.Status == billing.StatusOverdue {
				view.Stats.Overdue++
			} else if sub.Status == billing.StatusActive {
				view.Stats.Active++
			}
		}

		view.Rows = append(view.Rows, row)
		view.Accounts = append(view.Accounts, chargeAccount{
			ID:              account.ID,
			Name:            account.Name,
			Devices:         account.DeviceCount,
			UnitPriceValue:  row.UnitPriceValue,
			AmountValue:     row.ChargeValue,
			Currency:        row.Currency,
			PeriodDays:      row.DefaultPeriod,
			HasSubscription: row.HasSubscription,
		})
	}

	render(w, http.StatusOK, "dashboard", view)
}

func formatAmount(cents int64, currency string) string {
	return fmt.Sprintf("%s %.2f", currency, float64(cents)/100)
}

func daysUntil(due, now time.Time) int {
	return int(due.Sub(now).Hours() / 24)
}
