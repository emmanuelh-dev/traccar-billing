package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type sellerRow struct {
	Seller           billing.Seller
	CommissionValue  string
	CommissionLabel  string
	AccountCount     int
	DeviceCount      int
	MonthlyDisplay   string
	CommissionAmount string
}

type sellerOption struct {
	ID   int64
	Name string
}

type sellersView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Rows     []sellerRow
	Redirect string

	SessionExpired bool
}

func (s *Server) handleSellers(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	sellers, err := s.repo.ListSellers(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list sellers", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	accounts, err := s.repo.ListAccountsByTenant(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for sellers", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type totals struct {
		accounts int
		devices  int
		cents    int64
		currency string
	}
	bySeller := make(map[int64]*totals, len(sellers))

	for _, account := range accounts {
		if account.SellerID == 0 {
			continue
		}
		agg, ok := bySeller[account.SellerID]
		if !ok {
			agg = &totals{currency: defaultCurrency}
			bySeller[account.SellerID] = agg
		}
		agg.accounts++
		agg.devices += account.DeviceCount

		sub, err := s.repo.GetSubscriptionByAccountID(r.Context(), account.ID)
		switch {
		case errors.Is(err, billing.ErrNotFound):
		case err != nil:
			s.logger.Error("api: get subscription for sellers", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			agg.cents += billing.ChargeCents(sub, account.DeviceCount)
			agg.currency = sub.Currency
		}
	}

	view := sellersView{
		T:        t,
		Title:    t.SellersPageTtl,
		Active:   "sellers",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Redirect: "/sellers",

		SessionExpired: !tenant.HasValidSession(s.now()),
	}

	for _, seller := range sellers {
		row := sellerRow{
			Seller:           seller,
			CommissionValue:  strconv.FormatFloat(seller.CommissionPercent(), 'f', 2, 64),
			CommissionLabel:  strconv.FormatFloat(seller.CommissionPercent(), 'f', -1, 64) + "%",
			MonthlyDisplay:   formatAmount(0, defaultCurrency),
			CommissionAmount: formatAmount(0, defaultCurrency),
		}
		if agg, ok := bySeller[seller.ID]; ok {
			row.AccountCount = agg.accounts
			row.DeviceCount = agg.devices
			row.MonthlyDisplay = formatAmount(agg.cents, agg.currency)
			row.CommissionAmount = formatAmount(billing.CommissionCents(agg.cents, seller.CommissionBP), agg.currency)
		}
		view.Rows = append(view.Rows, row)
	}

	render(w, http.StatusOK, "sellers", view)
}

func (s *Server) parseSellerForm(r *http.Request) (billing.Seller, error) {
	var seller billing.Seller
	if err := r.ParseForm(); err != nil {
		return seller, errors.New("invalid form")
	}

	seller.Name = r.FormValue("name")
	if seller.Name == "" {
		return seller, errors.New("seller name is required")
	}
	seller.Email = r.FormValue("email")
	seller.Phone = r.FormValue("phone")
	seller.Note = r.FormValue("note")
	seller.Active = r.FormValue("active") != "0"

	if v := r.FormValue("commission"); v != "" {
		percent, err := strconv.ParseFloat(v, 64)
		if err != nil || percent < 0 || percent > 100 {
			return seller, errors.New("invalid commission")
		}
		seller.CommissionBP = int(percent * 100)
	}
	return seller, nil
}

func (s *Server) handleCreateSeller(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	seller, err := s.parseSellerForm(r)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	seller.TenantID = tenant.ID

	if _, err := s.repo.CreateSeller(r.Context(), seller); err != nil {
		s.logger.Error("api: create seller", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/sellers"), http.StatusSeeOther)
}

func (s *Server) handleUpdateSeller(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	sellerID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid seller id")
		return
	}

	seller, err := s.parseSellerForm(r)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	seller.ID = sellerID
	seller.TenantID = tenant.ID

	if _, err := s.repo.UpdateSeller(r.Context(), seller); errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "seller not found")
		return
	} else if err != nil {
		s.logger.Error("api: update seller", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/sellers"), http.StatusSeeOther)
}

func (s *Server) handleAssignSeller(w http.ResponseWriter, r *http.Request) {
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

	sellerID, _ := strconv.ParseInt(r.FormValue("seller_id"), 10, 64)

	err = s.repo.AssignAccountSeller(r.Context(), tenant.ID, accountID, sellerID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "account or seller not found")
		return
	}
	if err != nil {
		s.logger.Error("api: assign account seller", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
}

func (s *Server) sellerOptions(r *http.Request, tenantID int64) ([]sellerOption, map[int64]string, error) {
	sellers, err := s.repo.ListSellers(r.Context(), tenantID)
	if err != nil {
		return nil, nil, err
	}

	options := make([]sellerOption, 0, len(sellers))
	names := make(map[int64]string, len(sellers))
	for _, seller := range sellers {
		names[seller.ID] = seller.Name
		if seller.Active {
			options = append(options, sellerOption{ID: seller.ID, Name: seller.Name})
		}
	}
	return options, names, nil
}
