package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type dashboardStats struct {
	Total   int
	Active  int
	Overdue int
	Mirror  int
}

type accountRow struct {
	// T rides along because the row-action buttons are their own
	// template, invoked with the row as dot, so $ resolves to the row
	// rather than the page.
	T               uiStrings
	Account         billing.Account
	Subscription    billing.Subscription
	HasSubscription bool
	StatusLabel     string
	AmountDisplay   string
	AmountValue     string
	UnitPriceValue  string
	UnitPriceLabel  string
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
	IsAdmin         bool
	PaymentCount    int
}

// accountGroup is what the account table and card grid actually render.
// Ungrouped, the dashboard builds a single nameless group holding every
// row; grouped by seller, one per seller plus one for the unassigned. It
// carries its own copy of T so the shared row templates can be invoked
// with the group as their dot.
type accountGroup struct {
	T              uiStrings
	Name           string
	SellerID       int64
	Rows           []accountRow
	DeviceCount    int
	MonthlyDisplay string
}

func (g accountGroup) Summary() string {
	return fmt.Sprintf(g.T.GroupTotalFmt, len(g.Rows), g.DeviceCount, g.MonthlyDisplay)
}

// sortOption is one entry of the toolbar's ordering picker.
type sortOption struct {
	Key      string
	Label    string
	Selected bool
}

var dashboardSorts = map[string]bool{
	"name": true, "seller": true, "devices": true, "amount": true, "due": true, "status": true,
}

func dashboardSortOptions(t uiStrings, selected string) []sortOption {
	opts := []sortOption{
		{Key: "name", Label: t.ColAccount},
		{Key: "seller", Label: t.SellerLabel},
		{Key: "devices", Label: t.ColDevices},
		{Key: "amount", Label: t.ColAmount},
		{Key: "due", Label: t.ColNextDue},
		{Key: "status", Label: t.ColStatus},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Key == selected
	}
	return opts
}

// sortAccountRows orders the dashboard the way the toolbar asks. An account
// with no subscription has neither amount nor due date, so it sinks to the
// bottom of those orderings rather than passing for a zero.
func sortAccountRows(rows []accountRow, key string) {
	less := func(i, j int) bool { return rows[i].Account.Name < rows[j].Account.Name }
	switch key {
	case "seller":
		less = func(i, j int) bool {
			// Unassigned accounts sort last, matching how the seller groups do.
			if (rows[i].SellerName == "") != (rows[j].SellerName == "") {
				return rows[j].SellerName == ""
			}
			if rows[i].SellerName != rows[j].SellerName {
				return rows[i].SellerName < rows[j].SellerName
			}
			return rows[i].Account.Name < rows[j].Account.Name
		}
	case "devices":
		less = func(i, j int) bool {
			if rows[i].Account.DeviceCount != rows[j].Account.DeviceCount {
				return rows[i].Account.DeviceCount > rows[j].Account.DeviceCount
			}
			return rows[i].Account.Name < rows[j].Account.Name
		}
	case "amount":
		less = func(i, j int) bool {
			a, b := rowChargeCents(rows[i]), rowChargeCents(rows[j])
			if a != b {
				return a > b
			}
			return rows[i].Account.Name < rows[j].Account.Name
		}
	case "due":
		less = func(i, j int) bool {
			if rows[i].HasSubscription != rows[j].HasSubscription {
				return rows[i].HasSubscription
			}
			if !rows[i].Subscription.NextDueAt.Equal(rows[j].Subscription.NextDueAt) {
				return rows[i].Subscription.NextDueAt.Before(rows[j].Subscription.NextDueAt)
			}
			return rows[i].Account.Name < rows[j].Account.Name
		}
	case "status":
		less = func(i, j int) bool {
			a, b := statusRank(rows[i]), statusRank(rows[j])
			if a != b {
				return a < b
			}
			return rows[i].Account.Name < rows[j].Account.Name
		}
	}
	sort.SliceStable(rows, less)
}

func rowChargeCents(row accountRow) int64 {
	if !row.HasSubscription {
		return -1
	}
	return billing.ChargeCents(row.Subscription, row.Account.DeviceCount)
}

// statusRank puts what needs attention first: overdue, then suspended, then
// active, and accounts with no billing set up at all last.
func statusRank(row accountRow) int {
	if !row.HasSubscription {
		return 3
	}
	switch row.Subscription.Status {
	case billing.StatusOverdue:
		return 0
	case billing.StatusActive:
		return 2
	default:
		return 1
	}
}

type dashboardView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Stats    dashboardStats
	Rows     []accountRow
	Groups   []accountGroup
	Accounts []chargeAccount
	Sellers  []sellerOption
	Concepts []conceptOption
	Today    string
	Redirect string
	View     string
	Grouped  bool
	Sort     string
	Sorts    []sortOption
	// ShowMirror is the per-request override of the tenant's
	// HideMirrorAccounts setting, so mirror accounts can be inspected
	// without changing the saved default.
	ShowMirror bool

	SessionExpired bool
}

const defaultPeriodDays = 30

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	now := s.now()

	settings, err := s.repo.GetSettings(r.Context(), tenant.ID)
	if err != nil {
		s.logger.Error("api: get settings for dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

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

	conceptOpts, _, err := s.conceptOptions(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list concepts for dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	showMirror := !settings.HideMirrorAccounts || resolveToggle(w, r, mirrorCookieName)
	grouped := resolveToggle(w, r, groupCookieName)
	sortKey := resolveChoice(w, r, sortCookieName, dashboardSorts, "name")

	view := dashboardView{
		T:          t,
		Title:      t.DashboardTitle,
		Active:     "dashboard",
		Tenant:     tenant,
		Error:      r.URL.Query().Get("error"),
		Sellers:    sellerOpts,
		Concepts:   conceptOpts,
		Today:      now.Format(dueDateFormat),
		Redirect:   "/dashboard",
		View:       resolveView(w, r),
		Grouped:    grouped,
		Sort:       sortKey,
		Sorts:      dashboardSortOptions(t, sortKey),
		ShowMirror: showMirror,

		SessionExpired: !tenant.HasValidSession(time.Now()),
	}

	defaults := defaultsFromSettings(settings)

	for _, account := range accounts {
		if account.Mirror() {
			view.Stats.Mirror++
			if !showMirror {
				continue
			}
		}

		row := defaults
		row.Account = account
		row.IsAdmin = account.TraccarUserID == tenant.AdminTraccarUserID || strings.EqualFold(account.Email, "info@bysmax.com")
		row.DefaultDueDate = now.AddDate(0, 0, settings.PeriodDays).Format(dueDateFormat)
		row.SellerID = account.SellerID
		row.SellerName = sellerNames[account.SellerID]
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
			if sub.PerDevice() {
				row.UnitPriceLabel = formatAmount(sub.UnitPriceCents, sub.Currency)
			} else {
				row.UnitPriceLabel = t.FixedAmountLabel
			}
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

	sortAccountRows(view.Rows, sortKey)
	view.Groups = buildGroups(t, view.Rows, grouped, settings.Currency)

	render(w, http.StatusOK, "dashboard", view)
}

// buildGroups keeps the templates free of grouping logic: they always
// range over Groups, whether that is one nameless bucket or one per
// seller. Unassigned accounts sort last under an empty seller name.
func buildGroups(t uiStrings, rows []accountRow, grouped bool, currency string) []accountGroup {
	if len(rows) == 0 {
		return nil
	}
	// Every row carries T so the row-action template, which is invoked
	// with a row as its dot, can reach the translations.
	for i := range rows {
		rows[i].T = t
	}
	if !grouped {
		// The single bucket still needs its totals: the table foot shows them
		// whether or not the rows are split by seller.
		all := accountGroup{T: t, Rows: rows}
		var cents int64
		for _, row := range rows {
			all.DeviceCount += row.Account.DeviceCount
			if row.HasSubscription {
				cents += billing.ChargeCents(row.Subscription, row.Account.DeviceCount)
				currency = row.Subscription.Currency
			}
		}
		all.MonthlyDisplay = formatAmount(cents, currency)
		return []accountGroup{all}
	}

	bySeller := make(map[int64]*accountGroup)
	var cents = make(map[int64]int64)

	for _, row := range rows {
		group, ok := bySeller[row.SellerID]
		if !ok {
			name := row.SellerName
			if name == "" {
				name = t.NoSeller
			}
			group = &accountGroup{T: t, Name: name, SellerID: row.SellerID}
			bySeller[row.SellerID] = group
		}
		group.Rows = append(group.Rows, row)
		group.DeviceCount += row.Account.DeviceCount
		if row.HasSubscription {
			cents[row.SellerID] += billing.ChargeCents(row.Subscription, row.Account.DeviceCount)
			currency = row.Subscription.Currency
		}
	}

	groups := make([]accountGroup, 0, len(bySeller))
	for id, group := range bySeller {
		group.MonthlyDisplay = formatAmount(cents[id], currency)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		// Unassigned (seller 0) always sorts last, everything else by name.
		if (groups[i].SellerID == 0) != (groups[j].SellerID == 0) {
			return groups[j].SellerID == 0
		}
		return groups[i].Name < groups[j].Name
	})
	return groups
}

func formatAmount(cents int64, currency string) string {
	return fmt.Sprintf("%s %.2f", currency, float64(cents)/100)
}

func daysUntil(due, now time.Time) int {
	return int(due.Sub(now).Hours() / 24)
}
