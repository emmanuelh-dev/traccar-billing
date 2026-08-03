package api

import (
	"bytes"
	"strings"
	"testing"

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
		Accounts: []chargeAccount{
			{ID: 7, Name: "Cristian Palomo", Devices: 11, UnitPriceValue: "200.00", AmountValue: "2200.00", Currency: "MXN", PeriodDays: 30, HasSubscription: true},
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
