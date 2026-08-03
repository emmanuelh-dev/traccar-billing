package api

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func TestRenderDashboard(t *testing.T) {
	view := dashboardView{
		T:        stringsFor("es"),
		Title:    "Dashboard",
		Active:   "dashboard",
		Tenant:   billing.Tenant{Name: "gps.example.com", BaseURL: "https://gps.example.com/api"},
		Stats:    dashboardStats{Total: 2, Active: 1, Overdue: 1},
		Today:    "2026-08-02",
		Redirect: "/dashboard",
		Rows: []accountRow{
			{
				Account:         billing.Account{ID: 7, Name: "Cristian Palomo", Email: "c@example.com", DeviceCount: 11},
				Subscription:    billing.Subscription{Status: billing.StatusActive, Currency: "MXN"},
				HasSubscription: true,
				StatusLabel:     "al corriente",
				AmountDisplay:   "MXN 2200.00",
				UnitPriceValue:  "200.00",
				AmountValue:     "0.00",
				FlatFeeValue:    "0.00",
				ChargeValue:     "2200.00",
				Currency:        "MXN",
				DefaultDueDate:  "2026-09-02",
				DefaultPeriod:   30,
			},
			{
				Account:  billing.Account{ID: 8, Name: "Sin cobro", Email: "s@example.com"},
				Currency: "MXN",
			},
		},
		Accounts: []chargeAccount{
			{ID: 7, Name: "Cristian Palomo", Devices: 11, UnitPriceValue: "200.00", AmountValue: "2200.00", Currency: "MXN", PeriodDays: 30, HasSubscription: true},
		},
	}

	view.Groups = buildGroups(view.T, view.Rows, false, "MXN")

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "dashboard", view); err != nil {
		t.Fatalf("render dashboard: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="charge-dialog"`,
		`id="configure-dialog"`,
		`data-modal="charge"`,
		`data-modal="configure"`,
		`data-devices="11"`,
		`data-unit="200.00"`,
		"Cristian Palomo",
		`class="btn-icon"`,
		`aria-label="Configurar cobro"`,
		"<svg",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard output missing %q", want)
		}
	}
}

func TestRenderDashboardGroupedBySeller(t *testing.T) {
	rows := []accountRow{
		{
			Account:         billing.Account{ID: 1, Name: "Uno", DeviceCount: 3},
			Subscription:    billing.Subscription{Status: billing.StatusActive, Currency: "MXN", UnitPriceCents: 20000},
			HasSubscription: true,
			SellerID:        4,
			SellerName:      "Ana",
		},
		{Account: billing.Account{ID: 2, Name: "Dos", DeviceCount: 1}},
	}

	groups := buildGroups(stringsFor("es"), rows, true, "MXN")
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(groups))
	}
	if groups[0].Name != "Ana" {
		t.Errorf("first group = %q, want the named seller", groups[0].Name)
	}
	if groups[1].SellerID != 0 {
		t.Error("unassigned group should sort last")
	}
	if groups[0].DeviceCount != 3 {
		t.Errorf("group device count = %d, want 3", groups[0].DeviceCount)
	}
	if !strings.Contains(groups[0].Summary(), "MXN 600.00") {
		t.Errorf("group summary = %q, want the 3 x 200 total", groups[0].Summary())
	}

	view := dashboardView{
		T:        stringsFor("es"),
		Title:    "Dashboard",
		Active:   "dashboard",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Redirect: "/dashboard",
		Grouped:  true,
		Rows:     rows,
		Groups:   groups,
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "dashboard", view); err != nil {
		t.Fatalf("render grouped dashboard: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`class="group-header"`, "Ana", "/dashboard?group=0"} {
		if !strings.Contains(out, want) {
			t.Errorf("grouped dashboard missing %q", want)
		}
	}
}

func TestRenderDashboardDeleteAction(t *testing.T) {
	view := dashboardView{
		T:        stringsFor("es"),
		Title:    "Dashboard",
		Active:   "dashboard",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Redirect: "/dashboard",
		Rows:     []accountRow{{Account: billing.Account{ID: 9, Name: "Prueba", Email: "p@example.com"}}},
	}
	view.Groups = buildGroups(view.T, view.Rows, false, "MXN")

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "dashboard", view); err != nil {
		t.Fatalf("render dashboard: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-modal="account-delete"`,
		`id="account-delete-dialog"`,
		`class="btn-icon danger"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestRenderSettings(t *testing.T) {
	view := settingsView{
		T:              stringsFor("es"),
		Title:          "Ajustes",
		Active:         "settings",
		Saved:          true,
		Tenant:         billing.Tenant{Name: "gps.example.com"},
		Settings:       billing.DefaultSettings(1),
		Redirect:       "/settings",
		UnitPriceValue: "200.00",
		FlatFeeValue:   "0.00",
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "settings", view); err != nil {
		t.Fatalf("render settings: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`action="/settings"`,
		`name="hide_mirror"`,
		`name="billing_mode"`,
		`value="200.00"`,
		`class="form-ok"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("settings output missing %q", want)
		}
	}
	if !strings.Contains(out, `<option value="rolling" selected>`) {
		t.Error("settings should default to the rolling billing mode")
	}
}

func TestRenderPayments(t *testing.T) {
	view := paymentsView{
		T:        stringsFor("es"),
		Title:    "Pagos",
		Active:   "payments",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Today:    "2026-08-02",
		Total:    "MXN 2200.00",
		Redirect: "/payments",
		Rows: []tenantPaymentRow{
			{
				ID:            3,
				AccountID:     7,
				AccountName:   "Cristian Palomo",
				ConceptName:   "Mensualidad",
				DateDisplay:   "2026-08-02",
				DateValue:     "2026-08-02",
				AmountDisplay: "MXN 2200.00",
				AmountValue:   "2200.00",
				UnitPriceVal:  "200.00",
				Devices:       11,
				Method:        "cash",
			},
			{
				ID:            4,
				AccountName:   "Cristian Palomo",
				DateDisplay:   "2026-08-01",
				AmountDisplay: "MXN 200.00",
				Voided:        true,
				VoidReason:    "duplicado",
			},
		},
		Concepts: []conceptOption{
			{ID: 1, Name: "Instalación", AmountValue: "500.00", Currency: "MXN", Recurring: false},
		},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "payments", view); err != nil {
		t.Fatalf("render payments: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="charge-dialog"`,
		`id="payment-dialog"`,
		`id="void-dialog"`,
		`data-modal="payment"`,
		`data-modal="void"`,
		`data-payment="3"`,
		`class="voided"`,
		"duplicado",
		"Concepto",
		"Mensualidad",
		`data-recurring="false"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("payments output missing %q", want)
		}
	}

	if strings.Contains(out, `data-modal="void"\n            data-payment="4"`) {
		t.Error("payments output offers void on an already voided payment")
	}
	if !strings.Contains(out, `data-modal="delete"`) {
		t.Error("payments output is missing the delete action")
	}
}

func TestRenderConcepts(t *testing.T) {
	view := conceptsView{
		T:        stringsFor("es"),
		Title:    "Conceptos de Cobro",
		Active:   "concepts",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Redirect: "/concepts",
		Rows: []conceptRow{
			{
				Concept: billing.Concept{
					ID:          1,
					Name:        "Mensualidad",
					Slug:        "mensualidad",
					AmountCents: 20000,
					Currency:    "MXN",
					Recurring:   true,
					Active:      true,
					Note:        "Cobro mensual",
				},
				AmountDisplay:  "MXN 200.00",
				AmountValue:    "200.00",
				RecurringLabel: "Sí",
			},
		},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "concepts", view); err != nil {
		t.Fatalf("render concepts: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="concept-dialog"`,
		`id="concept-delete-dialog"`,
		`data-modal="concept-new"`,
		`data-modal="concept-edit"`,
		`data-modal="concept-delete"`,
		"Mensualidad",
		"mensualidad",
		"MXN 200.00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("concepts output missing %q", want)
		}
	}
}

func TestRenderDashboardCardView(t *testing.T) {
	view := dashboardView{
		T:        stringsFor("es"),
		Title:    "Dashboard",
		Active:   "dashboard",
		View:     "cards",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Today:    "2026-08-02",
		Redirect: "/dashboard",
		Rows: []accountRow{{
			Account:         billing.Account{ID: 7, Name: "Cristian Palomo", Email: "c@example.com", DeviceCount: 11},
			Subscription:    billing.Subscription{Status: billing.StatusActive, Currency: "MXN"},
			HasSubscription: true,
			StatusLabel:     "al corriente",
			AmountDisplay:   "MXN 2200.00",
			UnitPriceLabel:  "MXN 200.00",
			DefaultDueDate:  "2026-09-05",
			SellerName:      "Ana",
		}},
	}

	view.Groups = buildGroups(view.T, view.Rows, false, "MXN")

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "dashboard", view); err != nil {
		t.Fatalf("render dashboard cards: %v", err)
	}

	out := buf.String()
	for _, want := range []string{`class="card-grid"`, "account-card", "MXN 200.00", "Ana", "/dashboard?view=table"} {
		if !strings.Contains(out, want) {
			t.Errorf("card view missing %q", want)
		}
	}
	if strings.Contains(out, `class="table-scroll"`) {
		t.Error("card view still rendered the table")
	}
}

func TestRenderExpenses(t *testing.T) {
	view := expensesView{
		T:        stringsFor("es"),
		Title:    "Retiros",
		Active:   "expenses",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Redirect: "/expenses",
		Rows: []expenseRow{
			{
				Expense: billing.Expense{
					ID:          1,
					Category:    "Equipos GPS",
					AmountCents: 150000,
					Currency:    "MXN",
					SpentAt:     time.Now(),
					Method:      "transfer",
					Reference:   "REF123",
					Note:        "Compra de 5 equipos",
				},
				AmountDisplay: "MXN 1500.00",
				AmountValue:   "1500.00",
				DateDisplay:   "2026-08-02",
				DateValue:     "2026-08-02",
				SellerName:    "Ana",
				CategoryLabel: "Equipos GPS",
			},
		},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "expenses", view); err != nil {
		t.Fatalf("render expenses: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="expense-dialog"`,
		`id="expense-delete-dialog"`,
		`data-modal="expense-new"`,
		`data-modal="expense-edit"`,
		`data-modal="expense-delete"`,
		"Registrar retiro",
		"Equipos GPS",
		"MXN 1500.00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expenses output missing %q", want)
		}
	}
}

func TestRenderNavDrawer(t *testing.T) {
	view := dashboardView{
		T:      stringsFor("es"),
		Title:  "Panel",
		Active: "payments",
		Tenant: billing.Tenant{Name: "gps.example.com"},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "nav", view); err != nil {
		t.Fatalf("render nav: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="nav-toggle"`,
		`class="nav-scrim"`,
		`class="mobile-bar"`,
		`for="nav-toggle"`,
		`class="logout-form nav-logout"`,
		`gps.example.com`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("nav output missing %q", want)
		}
	}
}

func TestRenderAppointments(t *testing.T) {
	view := appointmentsView{
		T:        stringsFor("es"),
		Title:    "Agendas",
		Active:   "appointments",
		Tenant:   billing.Tenant{Name: "gps.example.com"},
		Today:    "2026-08-03",
		Redirect: "/appointments",
		Status:   "open",
		Rows: []appointmentRow{
			{
				Appointment: billing.Appointment{
					ID:          9,
					ClientName:  "Gabriel gzz",
					Phone:       "+52 81 1060 5924",
					Unit:        "Ford 350, Estaquita",
					Address:     "https://maps.app.goo.gl/abc",
					DeviceCount: 2,
					TimeWindow:  "1 o 2 pm",
					Status:      billing.AppointmentScheduled,
				},
				DateDisplay:   "2026-08-01",
				DateValue:     "2026-08-01",
				AmountDisplay: "MXN 1000.00",
				AmountValue:   "1000.00",
				StatusLabel:   "Agendada",
				Open:          true,
				Late:          true,
				WhatsApp:      []whatsappLink{{Phone: "+52 81 1060 5924", URL: "https://wa.me/528110605924?text=hola"}},
			},
		},
		Sellers:  []sellerOption{{ID: 1, Name: "Ana"}},
		Accounts: []chargeAccount{{ID: 4, Name: "BYSMAX"}},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "appointments", view); err != nil {
		t.Fatalf("render appointments: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`id="agenda-dialog"`,
		`id="agenda-status-dialog"`,
		`id="agenda-delete-dialog"`,
		`https://wa.me/528110605924?text=hola`,
		`data-modal="agenda-new"`,
		`data-modal="agenda-edit"`,
		`data-status="canceled"`,
		`Gabriel gzz`,
		`Atrasada`,
		`1 o 2 pm`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("appointments output missing %q", want)
		}
	}
}
