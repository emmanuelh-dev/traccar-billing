package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"

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

	// HasAPIToken and TokenHint describe the stored Traccar token without
	// ever putting it back on screen: it authenticates as the operator, so
	// rendering it would leak it into browser history and screenshots.
	HasAPIToken bool
	TokenHint   string

	SessionExpired bool
}

// tokenHint shows just enough of a token for the operator to tell two apart,
// and never enough to use one.
func tokenHint(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 6 {
		return "……"
	}
	return token[:3] + "……" + token[len(token)-3:]
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

		HasAPIToken: tenant.APIToken != "",
		TokenHint:   tokenHint(tenant.APIToken),

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

// handleSaveAPIToken stores a token the operator pasted in by hand, which is
// the way out when Traccar refuses to mint one (an older version, or a user
// without the permission).
//
// The token is validated against the tenant's own Traccar before being saved:
// storing a bad one would look like success and then quietly break the
// scheduler, which is the exact failure this whole feature exists to remove.
func (s *Server) handleSaveAPIToken(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}
	token := strings.TrimSpace(r.FormValue("api_token"))
	if token == "" {
		redirectPageError(w, r, "empty token")
		return
	}

	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		s.logger.Error("api: parse base url to verify token", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	if _, err := s.client.FetchServerInfo(r.Context(), baseURL, billing.Session{Token: token}); err != nil {
		s.logger.Warn("api: rejected pasted traccar token", "tenant_id", tenant.ID, "error", err)
		redirectPageError(w, r, "traccar rejected that token")
		return
	}

	if err := s.repo.UpdateTenantAPIToken(r.Context(), tenant.ID, token); err != nil {
		s.logger.Error("api: save traccar token", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

// handleGenerateAPIToken mints a token on demand, for the tenant that logged
// in before this existed or whose token was revoked in Traccar.
func (s *Server) handleGenerateAPIToken(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		s.logger.Error("api: parse base url to mint token", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	// Minting needs the login cookie: a token cannot be used to issue its own
	// replacement once Traccar has stopped honouring it.
	token, err := s.client.CreateToken(r.Context(), baseURL, billing.Session{Cookie: tenant.SessionCookie}, time.Now().Add(apiTokenTTL))
	if err != nil {
		s.logger.Warn("api: mint traccar token on demand", "tenant_id", tenant.ID, "error", err)
		redirectPageError(w, r, "traccar would not issue a token, sign in again or paste one")
		return
	}

	if err := s.repo.UpdateTenantAPIToken(r.Context(), tenant.ID, token); err != nil {
		s.logger.Error("api: store minted traccar token", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}

	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

// handleDeleteAPIToken forgets the token here. It does not revoke it in
// Traccar, which is done from Traccar itself.
func (s *Server) handleDeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	if err := s.repo.UpdateTenantAPIToken(r.Context(), tenant.ID, ""); err != nil {
		s.logger.Error("api: clear traccar token", "error", err)
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
