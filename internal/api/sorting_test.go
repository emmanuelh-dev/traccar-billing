package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func TestSortAccountRowsBySeller(t *testing.T) {
	rows := []accountRow{
		{Account: billing.Account{Name: "Zeta"}, SellerName: "Ana"},
		{Account: billing.Account{Name: "Beta"}},
		{Account: billing.Account{Name: "Alfa"}, SellerName: "Ana"},
		{Account: billing.Account{Name: "Gama"}, SellerName: "Beto"},
	}
	sortAccountRows(rows, "seller")

	want := []string{"Alfa", "Zeta", "Gama", "Beta"}
	for i, name := range want {
		if rows[i].Account.Name != name {
			t.Fatalf("row %d = %q, want %q (order %v)", i, rows[i].Account.Name, name, namesOf(rows))
		}
	}
}

func TestSortAccountRowsByStatusPutsOverdueFirst(t *testing.T) {
	rows := []accountRow{
		{Account: billing.Account{Name: "SinCobro"}},
		{Account: billing.Account{Name: "Activa"}, HasSubscription: true, Subscription: billing.Subscription{Status: billing.StatusActive}},
		{Account: billing.Account{Name: "Vencida"}, HasSubscription: true, Subscription: billing.Subscription{Status: billing.StatusOverdue}},
	}
	sortAccountRows(rows, "status")

	want := []string{"Vencida", "Activa", "SinCobro"}
	for i, name := range want {
		if rows[i].Account.Name != name {
			t.Fatalf("row %d = %q, want %q (order %v)", i, rows[i].Account.Name, name, namesOf(rows))
		}
	}
}

func TestSortAccountRowsByDueSinksAccountsWithoutBilling(t *testing.T) {
	early := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows := []accountRow{
		{Account: billing.Account{Name: "SinCobro"}},
		{Account: billing.Account{Name: "Tarde"}, HasSubscription: true, Subscription: billing.Subscription{NextDueAt: late}},
		{Account: billing.Account{Name: "Pronto"}, HasSubscription: true, Subscription: billing.Subscription{NextDueAt: early}},
	}
	sortAccountRows(rows, "due")

	want := []string{"Pronto", "Tarde", "SinCobro"}
	for i, name := range want {
		if rows[i].Account.Name != name {
			t.Fatalf("row %d = %q, want %q (order %v)", i, rows[i].Account.Name, name, namesOf(rows))
		}
	}
}

func namesOf(rows []accountRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.Account.Name
	}
	return out
}

func TestPaymentAccountTotalsSkipsVoided(t *testing.T) {
	rows := []tenantPaymentRow{
		{AccountName: "Uno", AmountValue: "100.00"},
		{AccountName: "Uno", AmountValue: "50.00"},
		{AccountName: "Dos", AmountValue: "900.00"},
		{AccountName: "Tres", AmountValue: "400.00"},
		{AccountName: "Tres", AmountValue: "999.00", Voided: true},
	}
	totals := paymentAccountTotals(rows, "MXN")

	if len(totals) != 3 {
		t.Fatalf("totals count = %d, want 3 (%+v)", len(totals), totals)
	}
	// Biggest first, so the page opens on whoever paid the most.
	if totals[0].Name != "Dos" || totals[0].AmountDisplay != "MXN 900.00" {
		t.Errorf("totals[0] = %+v, want Dos MXN 900.00", totals[0])
	}
	if totals[1].Name != "Tres" || totals[1].AmountDisplay != "MXN 400.00" {
		t.Errorf("totals[1] = %+v, want Tres MXN 400.00 (voided excluded)", totals[1])
	}
	if totals[2].Name != "Uno" || totals[2].AmountDisplay != "MXN 150.00" || totals[2].Count != 2 {
		t.Errorf("totals[2] = %+v, want Uno MXN 150.00 x2", totals[2])
	}
}

func TestPaymentAccountTotalsHiddenForFewAccounts(t *testing.T) {
	rows := []tenantPaymentRow{
		{AccountName: "Uno", AmountValue: "100.00"},
		{AccountName: "Dos", AmountValue: "100.00"},
	}
	if totals := paymentAccountTotals(rows, "MXN"); totals != nil {
		t.Errorf("totals = %+v, want none for two accounts", totals)
	}
}

func TestExpenseSellerTotals(t *testing.T) {
	rows := []expenseRow{
		{SellerName: "Beto", Expense: billing.Expense{AmountCents: 30000}},
		{SellerName: "Ana", Expense: billing.Expense{AmountCents: 10000}},
		{SellerName: "Ana", Expense: billing.Expense{AmountCents: 5000}},
	}
	totals := expenseSellerTotals(rows, "MXN")

	if len(totals) != 2 {
		t.Fatalf("totals count = %d, want 2 (%+v)", len(totals), totals)
	}
	if totals[0].Name != "Ana" || totals[0].AmountDisplay != "MXN 150.00" || totals[0].Count != 2 {
		t.Errorf("totals[0] = %+v, want Ana MXN 150.00 x2", totals[0])
	}
	if totals[1].Name != "Beto" || totals[1].AmountDisplay != "MXN 300.00" {
		t.Errorf("totals[1] = %+v, want Beto MXN 300.00", totals[1])
	}
}

func TestResolveChoiceRejectsUnknownValues(t *testing.T) {
	allowed := map[string]bool{"date": true, "amount": true}

	choose := func(target string) string {
		return resolveChoice(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil), "sort", allowed, "date")
	}
	if got := choose("/payments?sort=drop"); got != "date" {
		t.Errorf("resolveChoice with unknown param = %q, want the fallback", got)
	}
	if got := choose("/payments?sort=amount"); got != "amount" {
		t.Errorf("resolveChoice with known param = %q, want amount", got)
	}
}
