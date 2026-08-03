package api

import (
	"encoding/json"
	"errors"
	"net/http"
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
	AmountCents *int64     `json:"amount_cents"`
	Currency    string     `json:"currency"`
	PeriodDays  *int       `json:"period_days"`
	Note        string     `json:"note"`
	PaidAt      *time.Time `json:"paid_at"`
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
	if isJSON && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.respondPayFailure(w, r, isJSON, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
	switch {
	case errors.Is(err, billing.ErrNotFound):
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
	case err != nil:
		s.logger.Error("api: get subscription", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to get subscription")
		return
	default:
		if req.AmountCents != nil {
			sub.AmountCents = *req.AmountCents
		}
		if req.PeriodDays != nil {
			sub.PeriodDays = *req.PeriodDays
		}
		if req.Currency != "" {
			sub.Currency = req.Currency
		}
	}

	paidAt := time.Now().UTC()
	if req.PaidAt != nil {
		paidAt = *req.PaidAt
	}

	updatedSub := billing.ApplyPayment(sub, paidAt)
	updatedSub, err = s.repo.UpsertSubscription(r.Context(), updatedSub)
	if err != nil {
		s.logger.Error("api: upsert subscription", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to update subscription")
		return
	}

	payment, err := s.repo.RecordPayment(r.Context(), billing.Payment{
		SubscriptionID: updatedSub.ID,
		AmountCents:    updatedSub.AmountCents,
		Currency:       updatedSub.Currency,
		PaidAt:         paidAt,
		Note:           req.Note,
	})
	if err != nil {
		s.logger.Error("api: record payment", "error", err)
		s.respondPayFailure(w, r, isJSON, http.StatusInternalServerError, "failed to record payment")
		return
	}

	s.syncTraccarAccess(r.Context(), tenant, account, false)

	if !isJSON {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, accountDetail{
		Account:      account,
		Subscription: &updatedSub,
		Payments:     []billing.Payment{payment},
	})
}

func (s *Server) respondPayFailure(w http.ResponseWriter, r *http.Request, isJSON bool, status int, message string) {
	if isJSON {
		writeJSONError(w, status, message)
		return
	}
	redirectDashboardError(w, r, message)
}

func currencyOrDefault(currency string) string {
	if currency == "" {
		return "USD"
	}
	return currency
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}
