package api

import (
	"net/http"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type settingsView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Saved    bool
	Tenant   billing.Tenant
	Settings billing.Settings
	Redirect string

	UnitPriceValue string
	FlatFeeValue   string

	SessionExpired bool
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	settings, err := s.repo.GetSettings(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: get settings", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	render(w, http.StatusOK, "settings", settingsView{
		T:              t,
		Title:          t.SettingsPageTtl,
		Active:         "settings",
		Error:          r.URL.Query().Get("error"),
		Saved:          r.URL.Query().Get("saved") == "1",
		Tenant:         tenant,
		Settings:       settings,
		Redirect:       "/settings",
		UnitPriceValue: centsValue(settings.UnitPriceCents),
		FlatFeeValue:   centsValue(settings.FlatFeeCents),

		SessionExpired: !tenant.HasValidSession(s.now()),
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	unitPriceCents, err := parseAmountCents(orZero(r.FormValue("unit_price")))
	if err != nil {
		redirectPageError(w, r, "invalid unit price")
		return
	}
	flatFeeCents, err := parseAmountCents(orZero(r.FormValue("flat_fee")))
	if err != nil {
		redirectPageError(w, r, "invalid flat fee")
		return
	}

	current, err := s.repo.GetSettings(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: get settings for save", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	current.TenantID = tenant.ID
	current.BillingMode = billing.BillingMode(r.FormValue("billing_mode"))
	current.AnchorDay = clampDayOfMonth(optionalInt(r.FormValue("anchor_day"), defaultAnchorDay))
	current.DueDay = clampDayOfMonth(optionalInt(r.FormValue("due_day"), defaultDueDay))
	current.PeriodDays = optionalInt(r.FormValue("period_days"), defaultPeriodDays)
	current.GraceDays = optionalInt(r.FormValue("grace_days"), defaultGraceDays)
	current.Currency = currencyOrDefault(r.FormValue("currency"))
	current.UnitPriceCents = unitPriceCents
	current.FlatFeeCents = flatFeeCents
	current.MinDevices = optionalInt(r.FormValue("min_devices"), 0)
	current.HideMirrorAccounts = r.FormValue("hide_mirror") == "1"

	if _, err := s.repo.SaveSettings(r.Context(), current); err != nil {
		s.logger.Error("api: save settings", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

// defaultsFromSettings turns the tenant's stored defaults into the form
// values the configure modal pre-fills for an account that has no
// subscription yet.
func defaultsFromSettings(set billing.Settings) accountRow {
	return accountRow{
		Currency:       set.Currency,
		UnitPriceValue: centsValue(set.UnitPriceCents),
		FlatFeeValue:   centsValue(set.FlatFeeCents),
		AmountValue:    "0.00",
		ChargeValue:    centsValue(set.FlatFeeCents),
		MinDevices:     set.MinDevices,
		GraceDays:      set.GraceDays,
		DefaultPeriod:  set.PeriodDays,
		BillingMode:    string(set.BillingMode),
		AnchorDay:      set.AnchorDay,
		DueDay:         set.DueDay,
	}
}
