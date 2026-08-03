package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type expenseRow struct {
	Expense       billing.Expense
	AmountDisplay string
	AmountValue   string
	DateDisplay   string
	DateValue     string
	SellerName    string
	CategoryLabel string
}

// groupTotal is one subtotal line under a table, so a period can be read by
// who collected or spent it without leaving the page.
type groupTotal struct {
	Name          string
	Count         int
	AmountDisplay string
}

var expenseSorts = map[string]bool{"date": true, "seller": true, "category": true, "amount": true}

func expenseSortOptions(t uiStrings, selected string) []sortOption {
	opts := []sortOption{
		{Key: "date", Label: t.PaymentsColDate},
		{Key: "seller", Label: t.SellerLabel},
		{Key: "category", Label: t.CategoryLabel},
		{Key: "amount", Label: t.PaymentsColAmnt},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Key == selected
	}
	return opts
}

type expensesView struct {
	T            uiStrings
	Title        string
	Active       string
	Error        string
	Tenant       billing.Tenant
	Rows         []expenseRow
	Sellers      []sellerOption
	SellerTotals []groupTotal
	Sort         string
	Sorts        []sortOption
	Today        string
	Total        string
	Redirect     string

	Period     string
	FromValue  string
	ToValue    string
	RangeLabel string

	SessionExpired bool
}

func (s *Server) handleExpenses(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	period, filter := s.resolvePeriod(r)

	expenses, err := s.repo.ListExpenses(r.Context(), tenant.ID, filter)
	if err != nil {
		s.logger.Error("api: list expenses", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sellerOpts, sellerNames, err := s.sellerOptions(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list sellers for expenses", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sortKey := resolveChoice(w, r, expenseSortCookieName, expenseSorts, "date")

	view := expensesView{
		T:        t,
		Title:    t.ExpensesPageTtl,
		Active:   "expenses",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Sellers:  sellerOpts,
		Sort:     sortKey,
		Sorts:    expenseSortOptions(t, sortKey),
		Today:    s.now().Format(dueDateFormat),
		Redirect: r.URL.RequestURI(),

		Period:     period,
		FromValue:  r.URL.Query().Get("from"),
		ToValue:    r.URL.Query().Get("to"),
		RangeLabel: s.rangeLabel(filter),

		SessionExpired: !tenant.HasValidSession(time.Now()),
	}

	var totalCents int64
	currency := defaultCurrency
	for _, e := range expenses {
		totalCents += e.AmountCents
		if e.Currency != "" {
			currency = e.Currency
		}
		spentAt := e.SpentAt.In(s.loc)
		sellerName := sellerNames[e.SellerID]
		if sellerName == "" {
			sellerName = t.NoSeller
		}
		view.Rows = append(view.Rows, expenseRow{
			Expense:       e,
			AmountDisplay: formatAmount(e.AmountCents, e.Currency),
			AmountValue:   centsValue(e.AmountCents),
			DateDisplay:   spentAt.Format(dueDateFormat),
			DateValue:     spentAt.Format(dueDateFormat),
			SellerName:    sellerName,
			CategoryLabel: e.Category,
		})
	}
	view.Total = formatAmount(totalCents, currency)
	view.SellerTotals = expenseSellerTotals(view.Rows, currency)
	sortExpenseRows(view.Rows, sortKey)

	render(w, http.StatusOK, "expenses", view)
}

// expenseSellerTotals answers what each seller cost in the period, which is
// the reason to record a withdrawal against a seller in the first place. With
// a single bucket the table's grand total already says it, so it is skipped.
func expenseSellerTotals(rows []expenseRow, currency string) []groupTotal {
	if len(rows) == 0 {
		return nil
	}
	order := make([]string, 0, len(rows))
	counts := make(map[string]int)
	cents := make(map[string]int64)
	for _, row := range rows {
		if _, seen := counts[row.SellerName]; !seen {
			order = append(order, row.SellerName)
		}
		counts[row.SellerName]++
		cents[row.SellerName] += row.Expense.AmountCents
	}
	if len(order) < 2 {
		return nil
	}
	sort.Strings(order)

	totals := make([]groupTotal, 0, len(order))
	for _, name := range order {
		totals = append(totals, groupTotal{
			Name:          name,
			Count:         counts[name],
			AmountDisplay: formatAmount(cents[name], currency),
		})
	}
	return totals
}

func sortExpenseRows(rows []expenseRow, key string) {
	less := func(i, j int) bool { return rows[i].Expense.SpentAt.After(rows[j].Expense.SpentAt) }
	switch key {
	case "seller":
		less = func(i, j int) bool {
			if rows[i].SellerName != rows[j].SellerName {
				return rows[i].SellerName < rows[j].SellerName
			}
			return rows[i].Expense.SpentAt.After(rows[j].Expense.SpentAt)
		}
	case "category":
		less = func(i, j int) bool {
			if rows[i].Expense.Category != rows[j].Expense.Category {
				return rows[i].Expense.Category < rows[j].Expense.Category
			}
			return rows[i].Expense.SpentAt.After(rows[j].Expense.SpentAt)
		}
	case "amount":
		less = func(i, j int) bool { return rows[i].Expense.AmountCents > rows[j].Expense.AmountCents }
	}
	sort.SliceStable(rows, less)
}

func (s *Server) parseExpenseForm(r *http.Request, tenantID int64) (billing.Expense, error) {
	var expense billing.Expense
	if err := r.ParseForm(); err != nil {
		return expense, errors.New("invalid form")
	}

	amountRaw := r.FormValue("amount")
	if amountRaw == "" {
		return expense, errors.New("amount is required")
	}
	cents, err := parseAmountCents(amountRaw)
	if err != nil {
		return expense, errors.New("invalid amount")
	}
	expense.AmountCents = cents

	dateRaw := r.FormValue("spent_at")
	if dateRaw == "" {
		return expense, errors.New("date is required")
	}
	spentAt, err := time.ParseInLocation(dueDateFormat, dateRaw, s.loc)
	if err != nil {
		return expense, errors.New("invalid date")
	}
	expense.SpentAt = spentAt

	expense.Currency = r.FormValue("currency")
	if expense.Currency == "" {
		expense.Currency = "MXN"
	}

	if sellerIDStr := r.FormValue("seller_id"); sellerIDStr != "" && sellerIDStr != "0" {
		sellerID, err := strconv.ParseInt(sellerIDStr, 10, 64)
		if err != nil {
			return expense, errors.New("invalid seller id")
		}
		if sellerID > 0 {
			_, err := s.repo.GetSeller(r.Context(), tenantID, sellerID)
			if errors.Is(err, billing.ErrNotFound) {
				return expense, errors.New("seller not found")
			} else if err != nil {
				s.logger.Error("api: get seller for expense", "error", err)
				return expense, errors.New("internal error")
			}
			expense.SellerID = sellerID
		}
	}

	expense.Category = r.FormValue("category")
	expense.Method = r.FormValue("method")
	expense.Reference = r.FormValue("reference")
	expense.Note = r.FormValue("note")

	return expense, nil
}

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	expense, err := s.parseExpenseForm(r, tenant.ID)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	expense.TenantID = tenant.ID

	if _, err := s.repo.CreateExpense(r.Context(), expense); err != nil {
		s.logger.Error("api: create expense", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/expenses"), http.StatusSeeOther)
}

func (s *Server) handleUpdateExpense(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	expenseID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid expense id")
		return
	}

	expense, err := s.parseExpenseForm(r, tenant.ID)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	expense.ID = expenseID
	expense.TenantID = tenant.ID

	if _, err := s.repo.UpdateExpense(r.Context(), expense); errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "expense not found")
		return
	} else if err != nil {
		s.logger.Error("api: update expense", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/expenses"), http.StatusSeeOther)
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	expenseID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid expense id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	err = s.repo.DeleteExpense(r.Context(), tenant.ID, expenseID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "expense not found")
		return
	}
	if err != nil {
		s.logger.Error("api: delete expense", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/expenses"), http.StatusSeeOther)
}
