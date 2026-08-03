package api

import (
	"errors"
	"fmt"
	"net/http"
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
	ID            int64
	AccountID     int64
	AccountName   string
	DateDisplay   string
	DateValue     string
	AmountDisplay string
	AmountValue   string
	UnitPriceVal  string
	Devices       int
	Method        string
	Reference     string
	Note          string
	Voided        bool
	VoidReason    string
}

type paymentsView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Rows     []tenantPaymentRow
	Accounts []chargeAccount
	Today    string
	Total    string
	Redirect string
}

func (s *Server) handlePayments(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	payments, err := s.repo.ListPaymentsByTenant(r.Context(), tenant.ID)
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

	view := paymentsView{
		T:        t,
		Title:    t.PaymentsPageTtl,
		Active:   "payments",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Accounts: accounts,
		Today:    s.now().Format(dueDateFormat),
		Redirect: "/payments",
	}

	var totalCents int64
	currency := defaultCurrency
	for _, p := range payments {
		if !p.Voided() {
			totalCents += p.AmountCents
			currency = p.Currency
		}
		paidAt := p.PaidAt.In(s.loc)
		view.Rows = append(view.Rows, tenantPaymentRow{
			ID:            p.ID,
			AccountID:     p.AccountID,
			AccountName:   p.AccountName,
			DateDisplay:   paidAt.Format(dueDateFormat),
			DateValue:     paidAt.Format(dueDateFormat),
			AmountDisplay: formatAmount(p.AmountCents, p.Currency),
			AmountValue:   centsValue(p.AmountCents),
			UnitPriceVal:  centsValue(p.UnitPriceCents),
			Devices:       p.DeviceCount,
			Method:        p.Method,
			Reference:     p.Reference,
			Note:          p.Note,
			Voided:        p.Voided(),
			VoidReason:    p.VoidReason,
		})
	}
	view.Total = formatAmount(totalCents, currency)

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
	if req.DeviceCount != nil {
		payment.DeviceCount = *req.DeviceCount
	}
	if req.UnitPriceCents != nil {
		payment.UnitPriceCents = *req.UnitPriceCents
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

func centsValue(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
