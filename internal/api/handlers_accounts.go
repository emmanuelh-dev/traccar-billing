package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list accounts")
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

type accountDetail struct {
	billing.Account
	Subscription *billing.Subscription `json:"subscription,omitempty"`
	Payments     []billing.Payment     `json:"payments,omitempty"`
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	accountID, err := parseIDParam(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	account, err := s.repo.GetAccount(r.Context(), tenant.ID, accountID)
	if errors.Is(err, billing.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get account", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get account")
		return
	}

	detail := accountDetail{Account: account}

	sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
	switch {
	case errors.Is(err, billing.ErrNotFound):
		// no subscription yet, detail.Subscription stays nil
	case err != nil:
		s.logger.Error("api: get subscription", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get subscription")
		return
	default:
		detail.Subscription = &sub
		payments, err := s.repo.ListPaymentsBySubscription(r.Context(), sub.ID)
		if err != nil {
			s.logger.Error("api: list payments", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list payments")
			return
		}
		detail.Payments = payments
	}

	writeJSON(w, http.StatusOK, detail)
}

type payAccountRequest struct {
	// Items carries the charge's lines. An installation or an equipment sale
	// is several things at once, so the browser posts one row per line and
	// the total is the sum; the single-concept fields below stay for the
	// JSON API and for editing an already recorded payment.
	Items          []billing.PaymentItem
	ConceptID      *int64     `json:"concept_id"`
	AmountCents    *int64     `json:"amount_cents"`
	UnitPriceCents *int64     `json:"unit_price_cents"`
	DeviceCount    *int       `json:"device_count"`
	Currency       string     `json:"currency"`
	PeriodDays     *int       `json:"period_days"`
	Method         string     `json:"method"`
	Reference      string     `json:"reference"`
	Note           string     `json:"note"`
	PaidAt         *time.Time `json:"paid_at"`
}

func (s *Server) parsePayForm(r *http.Request) (payAccountRequest, error) {
	var req payAccountRequest
	if err := r.ParseForm(); err != nil {
		return req, fmt.Errorf("invalid form")
	}

	// The select is always submitted, so an empty value means "no concept"
	// and has to reach the handler as a zero rather than as a missing field:
	// that is what lets an edit clear a concept it had before.
	if r.Form.Has("concept_id") {
		var id int64
		if v := r.FormValue("concept_id"); v != "" {
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return req, fmt.Errorf("invalid concept")
			}
			id = parsed
		}
		req.ConceptID = &id
	}
	if v := r.FormValue("amount"); v != "" {
		cents, err := parseAmountCents(v)
		if err != nil {
			return req, fmt.Errorf("invalid amount")
		}
		req.AmountCents = &cents
	}
	if v := r.FormValue("unit_price"); v != "" {
		cents, err := parseAmountCents(v)
		if err != nil {
			return req, fmt.Errorf("invalid unit price")
		}
		req.UnitPriceCents = &cents
	}
	if v := r.FormValue("device_count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return req, fmt.Errorf("invalid device count")
		}
		req.DeviceCount = &n
	}
	if v := r.FormValue("paid_at"); v != "" {
		paidAt, err := time.ParseInLocation(dueDateFormat, v, s.loc)
		if err != nil {
			return req, fmt.Errorf("invalid payment date")
		}
		req.PaidAt = &paidAt
	}

	items, err := parseItemLines(r)
	if err != nil {
		return req, err
	}
	req.Items = items

	req.Currency = r.FormValue("currency")
	req.Method = r.FormValue("method")
	req.Reference = r.FormValue("reference")
	req.Note = r.FormValue("note")
	return req, nil
}

// parseItemLines reads the charge lines the modal posts as three parallel
// arrays. A row with no concept and no amount is one the operator added and
// left blank, so it is dropped instead of rejected.
func parseItemLines(r *http.Request) ([]billing.PaymentItem, error) {
	concepts := r.Form["item_concept_id"]
	if len(concepts) == 0 {
		return nil, nil
	}
	quantities := r.Form["item_qty"]
	prices := r.Form["item_price"]

	var items []billing.PaymentItem
	for i, raw := range concepts {
		var item billing.PaymentItem
		if raw != "" && raw != "0" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid concept")
			}
			item.ConceptID = id
		}
		item.Quantity = 1
		if i < len(quantities) && quantities[i] != "" {
			n, err := strconv.Atoi(quantities[i])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid quantity")
			}
			item.Quantity = n
		}
		if i < len(prices) && prices[i] != "" {
			cents, err := parseAmountCents(prices[i])
			if err != nil {
				return nil, fmt.Errorf("invalid line price")
			}
			item.UnitPriceCents = cents
		}
		if item.ConceptID == 0 && item.UnitPriceCents == 0 {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// handlePayAccount records a payment against an account's subscription and
// advances its next due date by one billing period. It serves both the
// dashboard's plain HTML "Registrar pago" form (redirects back on
// completion) and the JSON API (returns the updated account detail), based
// on the request's Content-Type.
func (s *Server) handlePayAccount(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	isJSON := strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")

	accountID, err := parseIDParam(r, "id")
	if err != nil {
		s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "invalid account id")
		return
	}

	account, err := s.repo.GetAccount(r.Context(), tenant.ID, accountID)
	if errors.Is(err, billing.ErrNotFound) {
		s.respondPayFailure(w, r, isJSON, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get account", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to get account")
		return
	}

	var req payAccountRequest
	if isJSON {
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "invalid request body")
				return
			}
		}
	} else {
		req, err = s.parsePayForm(r)
		if err != nil {
			s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, err.Error())
			return
		}
	}

	var conceptID int64
	if req.ConceptID != nil {
		conceptID = *req.ConceptID
	}
	var concept billing.Concept
	// A concept id arrives from the browser, so it has to be proven to
	// belong to this tenant before it lands on a payment row.
	if conceptID > 0 {
		var cErr error
		concept, cErr = s.repo.GetConcept(r.Context(), tenant.ID, conceptID)
		if cErr != nil {
			if errors.Is(cErr, billing.ErrNotFound) {
				s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "concept not found")
				return
			}
			s.logger.Error("api: get concept for payment", "error", cErr)
			s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "internal error")
			return
		}
	}

	sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
	hasSub := true
	if errors.Is(err, billing.ErrNotFound) {
		hasSub = false
	} else if err != nil {
		s.logger.Error("api: get subscription", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to get subscription")
		return
	}

	paidAt := s.now()
	if req.PaidAt != nil {
		paidAt = *req.PaidAt
	}

	if len(req.Items) > 0 {
		s.recordItemizedCharge(w, r, isJSON, tenant, account, sub, hasSub, req, paidAt)
		return
	}

	// One-off non-recurring charges (e.g. installation/uninstallation) do not
	// advance the billing cycle or alter subscription state.
	if conceptID > 0 && !concept.Recurring {
		amountCents := concept.AmountCents
		if req.AmountCents != nil {
			amountCents = *req.AmountCents
		}

		currency := sub.Currency
		if req.Currency != "" {
			currency = req.Currency
		}
		currency = currencyOrDefault(currency)

		var payment billing.Payment
		err = s.repo.WithTx(r.Context(), func(tx billing.Repository) error {
			var txErr error
			payment, txErr = tx.RecordPayment(r.Context(), billing.Payment{
				AccountID:      account.ID,
				SubscriptionID: sub.ID,
				ConceptID:      conceptID,
				AmountCents:    amountCents,
				UnitPriceCents: 0,
				DeviceCount:    0,
				Currency:       currency,
				Method:         req.Method,
				Reference:      req.Reference,
				PaidAt:         paidAt,
				Note:           req.Note,
			})
			return txErr
		})
		if err != nil {
			s.logger.Error("api: record payment", "error", err)
			s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to record payment")
			return
		}

		// Deliberately no syncTraccarAccess here: paying an installation
		// does not settle the monthly bill, so a suspended account stays
		// suspended until its subscription is actually paid.

		if !isJSON {
			http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
			return
		}
		writeJSON(w, http.StatusOK, accountDetail{
			Account:      account,
			Subscription: &sub,
			Payments:     []billing.Payment{payment},
		})
		return
	}

	if !hasSub {
		if req.AmountCents == nil || req.PeriodDays == nil {
			s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "account has no billing set up yet")
			return
		}
		sub = billing.Subscription{
			AccountID:   account.ID,
			Status:      billing.StatusActive,
			AmountCents: *req.AmountCents,
			Currency:    currencyOrDefault(req.Currency),
			PeriodDays:  *req.PeriodDays,
		}
	} else {
		if req.PeriodDays != nil {
			sub.PeriodDays = *req.PeriodDays
		}
		if req.Currency != "" {
			sub.Currency = req.Currency
		}
	}

	if req.UnitPriceCents != nil {
		sub.UnitPriceCents = *req.UnitPriceCents
	}

	devices := account.DeviceCount
	if req.DeviceCount != nil {
		devices = *req.DeviceCount
	}

	amountCents := billing.ChargeCents(sub, devices)
	if req.AmountCents != nil {
		amountCents = *req.AmountCents
	}

	var updatedSub billing.Subscription
	var payment billing.Payment
	err = s.repo.WithTx(r.Context(), func(tx billing.Repository) error {
		var txErr error
		updatedSub, txErr = tx.UpsertSubscription(r.Context(), billing.ApplyPayment(sub, paidAt))
		if txErr != nil {
			return txErr
		}
		payment, txErr = tx.RecordPayment(r.Context(), billing.Payment{
			AccountID:      account.ID,
			SubscriptionID: updatedSub.ID,
			ConceptID:      conceptID,
			AmountCents:    amountCents,
			UnitPriceCents: sub.UnitPriceCents,
			DeviceCount:    devices,
			Currency:       updatedSub.Currency,
			Method:         req.Method,
			Reference:      req.Reference,
			PaidAt:         paidAt,
			Note:           req.Note,
		})
		return txErr
	})
	if err != nil {
		s.logger.Error("api: record payment", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to record payment")
		return
	}

	s.syncTraccarAccess(r.Context(), tenant, account, false)

	if !isJSON {
		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, accountDetail{
		Account:      account,
		Subscription: &updatedSub,
		Payments:     []billing.Payment{payment},
	})
}

// recordItemizedCharge books a charge made of several lines. Only a line
// whose concept is recurring settles the monthly bill, so a payment that is
// pure installation or equipment leaves the billing cycle and the Traccar
// access exactly where they were, and does not even need a subscription.
func (s *Server) recordItemizedCharge(w http.ResponseWriter, r *http.Request, isJSON bool, tenant billing.Tenant, account billing.Account, sub billing.Subscription, hasSub bool, req payAccountRequest, paidAt time.Time) {
	items := req.Items
	renews := false
	var recurringLine billing.PaymentItem
	// conceptLine is the first line that names a concept. The payments page
	// reads the concept from the payment row, so a charge with no recurring
	// line still needs one there or the whole column shows up empty.
	var conceptLine int64
	for i := range items {
		recurring := false
		if items[i].ConceptID > 0 {
			concept, err := s.repo.GetConcept(r.Context(), tenant.ID, items[i].ConceptID)
			if errors.Is(err, billing.ErrNotFound) {
				s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "concept not found")
				return
			}
			if err != nil {
				s.logger.Error("api: get concept for charge line", "error", err)
				s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "internal error")
				return
			}
			items[i].Description = concept.Name
			recurring = concept.Recurring
		} else {
			// A line with no concept is the plain monthly fee, which is how the
			// dashboard has always recorded a renewal.
			recurring = true
		}
		items[i].AmountCents = int64(items[i].Quantity) * items[i].UnitPriceCents
		if recurring && !renews {
			renews = true
			recurringLine = items[i]
		}
		if conceptLine == 0 && items[i].ConceptID > 0 {
			conceptLine = items[i].ConceptID
		}
	}

	total := billing.ItemsTotal(items)
	if total <= 0 {
		s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "the charge has no amount")
		return
	}
	if renews && !hasSub {
		s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "account has no billing set up yet")
		return
	}

	currency := currencyOrDefault(firstNonEmpty(req.Currency, sub.Currency))
	payment := billing.Payment{
		AccountID:   account.ID,
		AmountCents: total,
		Currency:    currency,
		Method:      req.Method,
		Reference:   req.Reference,
		PaidAt:      paidAt,
		Note:        req.Note,
	}
	payment.ConceptID = conceptLine
	if renews {
		// The recurring line is what the per-device breakdown on the payments
		// page describes, so it is the one that lands on the payment row.
		payment.ConceptID = recurringLine.ConceptID
		payment.DeviceCount = recurringLine.Quantity
		payment.UnitPriceCents = recurringLine.UnitPriceCents
	}

	var updatedSub billing.Subscription
	err := s.repo.WithTx(r.Context(), func(tx billing.Repository) error {
		if renews {
			var txErr error
			updatedSub, txErr = tx.UpsertSubscription(r.Context(), billing.ApplyPayment(sub, paidAt))
			if txErr != nil {
				return txErr
			}
			payment.SubscriptionID = updatedSub.ID
		}
		recorded, txErr := tx.RecordPayment(r.Context(), payment)
		if txErr != nil {
			return txErr
		}
		payment = recorded
		payment.Items, txErr = tx.ReplacePaymentItems(r.Context(), recorded.ID, items)
		return txErr
	})
	if err != nil {
		s.logger.Error("api: record itemized payment", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to record payment")
		return
	}

	detail := accountDetail{Account: account, Payments: []billing.Payment{payment}}
	if renews {
		s.syncTraccarAccess(r.Context(), tenant, account, false)
		detail.Subscription = &updatedSub
	}

	if !isJSON {
		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) respondPayFailure(w http.ResponseWriter, r *http.Request, isJSON bool, status int, message string) {
	if isJSON {
		writeJSONError(w, status, message)
		return
	}
	redirectPageError(w, r, message)
}

// handleDeleteAccount removes the account here and the matching user in
// Traccar. Traccar goes first on purpose: deleting only locally would be
// undone by the next scheduler sync, which recreates an account for every
// user the server still lists.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	accountID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid account id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	account, err := s.repo.GetAccount(r.Context(), tenant.ID, accountID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "account not found")
		return
	}
	if err != nil {
		s.logger.Error("api: get account for delete", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	// Deleting the user whose session drives every Traccar call would lock
	// the tenant out of its own server.
	if account.TraccarUserID == tenant.AdminTraccarUserID || strings.EqualFold(account.Email, "info@bysmax.com") {
		redirectPageError(w, r, "cannot delete the tenant's own admin account")
		return
	}

	if err := s.deleteTraccarUser(r.Context(), tenant, account); err != nil {
		s.logger.Error("api: delete traccar user", "account_id", account.ID, "traccar_user_id", account.TraccarUserID, "error", err)
		redirectPageError(w, r, "could not delete the user in Traccar")
		return
	}

	if err := s.repo.DeleteAccount(r.Context(), tenant.ID, accountID); err != nil {
		s.logger.Error("api: delete account", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	s.logger.Info("api: deleted account", "tenant_id", tenant.ID, "account_id", accountID, "traccar_user_id", account.TraccarUserID)

	http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
}

func (s *Server) deleteTraccarUser(ctx context.Context, tenant billing.Tenant, account billing.Account) error {
	if !tenant.HasValidSession(time.Now()) {
		return errors.New("traccar session expired")
	}
	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	session := tenant.TraccarSession()
	return s.client.DeleteUser(ctx, baseURL, session, account.TraccarUserID)
}

func currencyOrDefault(currency string) string {
	if currency == "" {
		return defaultCurrency
	}
	return currency
}

func redirectTarget(r *http.Request, fallback string) string {
	target := r.FormValue("redirect_to")
	if strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "//") {
		return target
	}
	return fallback
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}
