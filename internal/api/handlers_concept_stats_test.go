package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// seedConceptStats records two charges in August 2026 against the same account
// so the concepts page has something to chart.
func seedConceptStats(t *testing.T, srv *Server, repo billing.Repository) ([]*http.Cookie, billing.Concept) {
	t.Helper()
	ctx := context.Background()

	srv.client = &loginStubClient{users: map[string]billing.TraccarUser{
		"owner@example.com": {ID: 7, Name: "Owner", Email: "owner@example.com"},
	}}
	cookies := loginAs(t, srv.Router(), "https://gps.example.com", "owner@example.com")

	tenants, err := repo.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants() error = %v", err)
	}
	tenant := tenants[0]

	account, err := repo.UpsertAccount(ctx, billing.Account{
		TenantID: tenant.ID, TraccarUserID: 42, Name: "Cliente Uno", Email: "uno@example.com", DeviceCount: 3,
	})
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}

	// A new tenant already comes with its default concepts, so the charges are
	// booked against one of those rather than against a duplicate.
	concepts, err := repo.ListConcepts(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListConcepts() error = %v", err)
	}
	if len(concepts) == 0 {
		t.Fatal("tenant has no seeded concepts")
	}
	concept := concepts[0]

	for _, at := range []time.Time{
		time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
	} {
		if _, err := repo.RecordPayment(ctx, billing.Payment{
			AccountID:   account.ID,
			ConceptID:   concept.ID,
			AmountCents: 150000,
			Currency:    "MXN",
			Method:      "cash",
			PaidAt:      at,
		}); err != nil {
			t.Fatalf("RecordPayment() error = %v", err)
		}
	}

	return cookies, concept
}

func getConcepts(t *testing.T, srv *Server, cookies []*http.Cookie, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/concepts?"+query, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}
	return w
}

func TestConceptsPageChartsTheSelectedMonth(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, concept := seedConceptStats(t, srv, repo)

	body := getConcepts(t, srv, cookies, "year=2026&month=8&by=day").Body.String()

	if !strings.Contains(body, "MXN 3000.00") {
		t.Errorf("page does not show the period total MXN 3000.00")
	}
	if !strings.Contains(body, concept.Name) {
		t.Errorf("page does not name the concept behind the income")
	}
	// August has 31 days and every one of them is a column, including the ones
	// with no income; a chart that skips them misreads the shape of the month.
	if got := strings.Count(body, `class="chart-col"`); got != 31 {
		t.Errorf("chart columns = %d, want 31", got)
	}
}

func TestConceptsPageMonthlyGranularityCoversTheYear(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, _ := seedConceptStats(t, srv, repo)

	body := getConcepts(t, srv, cookies, "year=2026&month=all&by=month").Body.String()

	if got := strings.Count(body, `class="chart-col"`); got != 12 {
		t.Errorf("chart columns = %d, want 12", got)
	}
	if !strings.Contains(body, "MXN 3000.00") {
		t.Errorf("the yearly view lost the income recorded in August")
	}
}

// A month with nothing recorded must say so rather than draw an empty chart
// the operator would read as a rendering failure.
func TestConceptsPageEmptyPeriod(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	cookies, _ := seedConceptStats(t, srv, repo)

	body := getConcepts(t, srv, cookies, "year=2026&month=2&by=day").Body.String()

	if strings.Contains(body, `class="chart-col"`) {
		t.Errorf("a period with no income still drew a chart")
	}
	if !strings.Contains(body, stringsFor("es").NoIncomeInPeriod) {
		t.Errorf("page does not explain that the period has no income")
	}
}
