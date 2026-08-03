package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type chargeAccount struct {
	ID              int64
	Name            string
	Devices         int
	UnitPriceValue  string
	AmountValue     string
	Currency        string
	PeriodDays      int
	HasSubscription bool
}

type tenantPaymentRow struct {
	ID               int64
	AccountID        int64
	AccountName      string
	ConceptID        int64
	ConceptName      string
	ConceptRecurring bool
	DateDisplay      string
	DateValue        string
	AmountDisplay    string
	AmountValue      string
	UnitPriceVal     string
	Currency         string
	Breakdown        string
	Devices          int
	Method           string
	Reference        string
	Note             string
	Voided           bool
	VoidReason       string
}

type paymentsView struct {
	T              uiStrings
	Title          string
	Active         string
	Error          string
	Tenant         billing.Tenant
	Rows           []tenantPaymentRow
	Accounts       []chargeAccount
	Concepts       []conceptOption
	Today          string
	Total          string
	TotalWithdrawn string
	NetTotal       string
	ShowNet        bool
	Redirect       string

	Period        string
	FromValue     string
	ToValue       string
	FilterAccount int64
	RangeLabel    string
	VoidedCount   int

	SessionExpired bool
}

// resolvePeriod turns the ?period= shortcut into a half-open [from, to)
// range in the tenant's timezone. "range" reads the explicit dates and
// anything unrecognised means no filter at all.
func (s *Server) resolvePeriod(r *http.Request) (string, billing.PaymentFilter) {
	now := s.now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.loc)

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "current"
	}

	switch period {
	case "current":
		return period, billing.PaymentFilter{From: monthStart, To: monthStart.AddDate(0, 1, 0)}
	case "previous":
		return period, billing.PaymentFilter{From: monthStart.AddDate(0, -1, 0), To: monthStart}
	case "range":
		var filter billing.PaymentFilter
		if from, err := time.ParseInLocation(dueDateFormat, r.URL.Query().Get("from"), s.loc); err == nil {
			filter.From = from
		}
		if to, err := time.ParseInLocation(dueDateFormat, r.URL.Query().Get("to"), s.loc); err == nil {
			filter.To = to.AddDate(0, 0, 1)
		}
		return period, filter
	default:
		return "all", billing.PaymentFilter{}
	}
}

func (s *Server) rangeLabel(filter billing.PaymentFilter) string {
	if filter.From.IsZero() && filter.To.IsZero() {
		return ""
	}
	from, to := "…", "…"
	if !filter.From.IsZero() {
		from = filter.From.In(s.loc).Format(dueDateFormat)
	}
	if !filter.To.IsZero() {
		to = filter.To.In(s.loc).AddDate(0, 0, -1).Format(dueDateFormat)
	}
	return from + " → " + to
}

func (s *Server) handlePayments(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	period, filter := s.resolvePeriod(r)
	filter.AccountID, _ = strconv.ParseInt(r.URL.Query().Get("account"), 10, 64)

	payments, err := s.repo.ListPaymentsByTenant(r.Context(), tenant.ID, filter)
	if err != nil {
		s.logger.Error("api: list tenant payments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	accounts, err := s.chargeAccounts(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for payments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	conceptOpts, _, err := s.conceptOptions(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list concepts for payments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Withdrawals belong to the tenant, not to an account, so the net only
	// means something while the list is showing every account's payments.
	var totalWithdrawnCents int64
	showNet := filter.AccountID == 0
	if showNet {
		expenses, err := s.repo.ListExpenses(r.Context(), tenant.ID, filter)
		if err != nil {
			s.logger.Error("api: list expenses for payments net", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, e := range expenses {
			totalWithdrawnCents += e.AmountCents
		}
	}

	view := paymentsView{
		T:        t,
		Title:    t.PaymentsPageTtl,
		Active:   "payments",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Accounts: accounts,
		Concepts: conceptOpts,
		Today:    s.now().Format(dueDateFormat),
		Redirect: r.URL.RequestURI(),

		Period:        period,
		FromValue:     r.URL.Query().Get("from"),
		ToValue:       r.URL.Query().Get("to"),
		FilterAccount: filter.AccountID,
		RangeLabel:    s.rangeLabel(filter),

		SessionExpired: !tenant.HasValidSession(time.Now()),
	}

	var totalCents int64
	currency := defaultCurrency
	for _, p := range payments {
		if p.Voided() {
			view.VoidedCount++
		} else {
			totalCents += p.AmountCents
			currency = p.Currency
		}
		paidAt := p.PaidAt.In(s.loc)
		view.Rows = append(view.Rows, tenantPaymentRow{
			ID:               p.ID,
			AccountID:        p.AccountID,
			AccountName:      p.AccountName,
			ConceptID:        p.ConceptID,
			ConceptName:      p.ConceptName,
			ConceptRecurring: p.ConceptRecurring,
			DateDisplay:      paidAt.Format(dueDateFormat),
			DateValue:        paidAt.Format(dueDateFormat),
			AmountDisplay:    formatAmount(p.AmountCents, p.Currency),
			AmountValue:      centsValue(p.AmountCents),
			UnitPriceVal:     centsValue(p.UnitPriceCents),
			Currency:         p.Currency,
			Breakdown:        breakdownLabel(t, p.DeviceCount, p.UnitPriceCents, p.Currency),
			Devices:          p.DeviceCount,
			Method:           p.Method,
			Reference:        p.Reference,
			Note:             p.Note,
			Voided:           p.Voided(),
			VoidReason:       p.VoidReason,
		})
	}
	view.Total = formatAmount(totalCents, currency)
	view.ShowNet = showNet
	view.TotalWithdrawn = formatAmount(totalWithdrawnCents, currency)
	view.NetTotal = formatAmount(totalCents-totalWithdrawnCents, currency)

	render(w, http.StatusOK, "payments", view)
}

func (s *Server) chargeAccounts(r *http.Request, tenantID int64) ([]chargeAccount, error) {
	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenantID)
	if err != nil {
		return nil, err
	}

	var out []chargeAccount
	for _, account := range accounts {
		entry := chargeAccount{
			ID:             account.ID,
			Name:           account.Name,
			Devices:        account.DeviceCount,
			Currency:       defaultCurrency,
			PeriodDays:     defaultPeriodDays,
			UnitPriceValue: "0.00",
			AmountValue:    "0.00",
		}

		sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
		switch {
		case errors.Is(err, billing.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			entry.HasSubscription = true
			entry.Currency = sub.Currency
			entry.PeriodDays = sub.PeriodDays
			entry.UnitPriceValue = centsValue(sub.UnitPriceCents)
			entry.AmountValue = centsValue(billing.ChargeCents(sub, account.DeviceCount))
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *Server) handleEditPayment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	paymentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid payment id")
		return
	}

	req, err := s.parsePayForm(r)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	if req.AmountCents == nil {
		redirectPageError(w, r, "amount is required")
		return
	}

	payment, err := s.repo.GetPayment(r.Context(), tenant.ID, paymentID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "payment not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get payment for edit", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	if payment.Voided() {
		redirectPageError(w, r, "a voided payment cannot be edited")
		return
	}

	payment.AmountCents = *req.AmountCents
	var isNonRecurring bool
	if req.ConceptID != nil {
		if *req.ConceptID > 0 {
			c, err := s.repo.GetConcept(r.Context(), tenant.ID, *req.ConceptID)
			if err != nil {
				if errors.Is(err, billing.ErrNotFound) {
					redirectPageError(w, r, "concept not found")
					return
				}
				s.logger.Error("api: get concept for payment edit", "error", err)
				redirectPageError(w, r, "internal error")
				return
			}
			isNonRecurring = !c.Recurring
		}
		payment.ConceptID = *req.ConceptID
	}
	if isNonRecurring {
		payment.DeviceCount = 0
		payment.UnitPriceCents = 0
	} else {
		if req.DeviceCount != nil {
			payment.DeviceCount = *req.DeviceCount
		}
		if req.UnitPriceCents != nil {
			payment.UnitPriceCents = *req.UnitPriceCents
		}
	}
	if req.PaidAt != nil {
		payment.PaidAt = *req.PaidAt
	}
	if req.Currency != "" {
		payment.Currency = req.Currency
	}
	payment.Method = req.Method
	payment.Reference = req.Reference
	payment.Note = req.Note

	if _, err := s.repo.UpdatePayment(r.Context(), payment); err != nil {
		s.logger.Error("api: update payment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, redirectTarget(r, "/payments"), http.StatusSeeOther)
}

func (s *Server) handleVoidPayment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	paymentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid payment id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	_, err = s.repo.GetPayment(r.Context(), tenant.ID, paymentID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "payment not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get payment for void", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	err = s.repo.VoidPayment(r.Context(), paymentID, time.Now().UTC(), r.FormValue("void_reason"))
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "payment is already voided")
		return
	}
	if err != nil {
		s.logger.Error("api: void payment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, redirectTarget(r, "/payments"), http.StatusSeeOther)
}

func (s *Server) handleDeletePayment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	paymentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid payment id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	err = s.repo.DeletePayment(r.Context(), tenant.ID, paymentID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "payment not found")
		return
	}
	if err != nil {
		s.logger.Error("api: delete payment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	s.logger.Info("api: deleted payment", "tenant_id", tenant.ID, "payment_id", paymentID)

	http.Redirect(w, r, redirectTarget(r, "/payments"), http.StatusSeeOther)
}

func centsValue(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func breakdownLabel(t uiStrings, devices int, unitPriceCents int64, currency string) string {
	if devices <= 0 || unitPriceCents <= 0 {
		return ""
	}
	return fmt.Sprintf("%d %s x %s %s %s", devices, t.DevicesWord, currency, centsValue(unitPriceCents), t.EachWord)
}
