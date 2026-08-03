package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

func postAgenda(t *testing.T, srv *Server, tenantID int64, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cookieVal, err := srv.signer.encode(tenantID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode session cookie error = %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieVal})

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func TestAppointmentLifecycle(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	ctx := context.Background()
	tenant, _, _ := setupTestTenantAndAccount(t, repo)

	form := url.Values{}
	form.Set("client_name", "Gabriel gzz")
	form.Set("scheduled_on", "2026-08-01")
	form.Set("time_window", "1 o 2 pm")
	form.Set("phone", "+52 81 1060 5924")
	form.Set("unit", "Ford 350, Estaquita")
	form.Set("address", "https://maps.app.goo.gl/3ar1P3Vrmm6nsgoP7")
	form.Set("device_count", "2")
	form.Set("amount", "1000.00")

	if w := postAgenda(t, srv, tenant.ID, "/appointments", form); w.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d (body %s)", w.Code, http.StatusSeeOther, w.Body.String())
	}

	appointments, err := repo.ListAppointments(ctx, tenant.ID, billing.AppointmentFilter{})
	if err != nil {
		t.Fatalf("ListAppointments error = %v", err)
	}
	if len(appointments) != 1 {
		t.Fatalf("appointments count = %d, want 1", len(appointments))
	}
	created := appointments[0]
	if created.Status != billing.AppointmentScheduled {
		t.Errorf("Status = %q, want scheduled", created.Status)
	}
	if created.DeviceCount != 2 || created.AmountCents != 100000 {
		t.Errorf("DeviceCount/AmountCents = %d/%d, want 2/100000", created.DeviceCount, created.AmountCents)
	}
	if created.TimeWindow != "1 o 2 pm" {
		t.Errorf("TimeWindow = %q, want %q", created.TimeWindow, "1 o 2 pm")
	}

	// Closing it stamps ClosedAt and keeps the reason.
	closeForm := url.Values{}
	closeForm.Set("status", "done")
	closeForm.Set("outcome", "instalado")
	target := "/appointments/" + strconv.FormatInt(created.ID, 10) + "/status"
	if w := postAgenda(t, srv, tenant.ID, target, closeForm); w.Code != http.StatusSeeOther {
		t.Fatalf("close status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	closed, err := repo.GetAppointment(ctx, tenant.ID, created.ID)
	if err != nil {
		t.Fatalf("GetAppointment error = %v", err)
	}
	if closed.Status != billing.AppointmentDone {
		t.Errorf("Status = %q, want done", closed.Status)
	}
	if closed.ClosedAt.IsZero() {
		t.Error("ClosedAt is zero, want the closing stamp")
	}
	if closed.Outcome != "instalado" {
		t.Errorf("Outcome = %q, want %q", closed.Outcome, "instalado")
	}

	// Reopening clears the stamp so a mistaken close leaves no date behind.
	reopenForm := url.Values{}
	reopenForm.Set("status", "scheduled")
	if w := postAgenda(t, srv, tenant.ID, target, reopenForm); w.Code != http.StatusSeeOther {
		t.Fatalf("reopen status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	reopened, err := repo.GetAppointment(ctx, tenant.ID, created.ID)
	if err != nil {
		t.Fatalf("GetAppointment error = %v", err)
	}
	if !reopened.Open() {
		t.Errorf("Status = %q, want scheduled", reopened.Status)
	}
	if !reopened.ClosedAt.IsZero() {
		t.Errorf("ClosedAt = %v, want cleared", reopened.ClosedAt)
	}

	// And cancelling takes it out of the open list.
	cancelForm := url.Values{}
	cancelForm.Set("status", "canceled")
	if w := postAgenda(t, srv, tenant.ID, target, cancelForm); w.Code != http.StatusSeeOther {
		t.Fatalf("cancel status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	open, err := repo.ListAppointments(ctx, tenant.ID, billing.AppointmentFilter{Status: billing.AppointmentScheduled})
	if err != nil {
		t.Fatalf("ListAppointments error = %v", err)
	}
	if len(open) != 0 {
		t.Errorf("open appointments = %d, want 0", len(open))
	}
}

func TestCreateAppointmentRequiresClientAndDate(t *testing.T) {
	srv, repo, _ := newTestServer(t)
	tenant, _, _ := setupTestTenantAndAccount(t, repo)

	form := url.Values{}
	form.Set("scheduled_on", "2026-08-01")

	w := postAgenda(t, srv, tenant.ID, "/appointments", form)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("Location = %q, want an error param for a nameless visit", loc)
	}
}

func TestWhatsAppLinks(t *testing.T) {
	tr := stringsFor("es")
	a := billing.Appointment{
		ClientName: "Gabriel gzz",
		Phone:      "+52 81 1060 5924, 8112345678",
		Unit:       "Ford 350",
		Address:    "https://maps.app.goo.gl/abc",
		TimeWindow: "1 o 2 pm",
	}

	links := whatsappLinks(tr, a, "2026-08-01")
	if len(links) != 2 {
		t.Fatalf("links = %d, want 2 (%+v)", len(links), links)
	}
	if !strings.HasPrefix(links[0].URL, "https://wa.me/528110605924?text=") {
		t.Errorf("links[0].URL = %q, want the digits-only international number", links[0].URL)
	}
	// A local 10-digit number gets the country code it was written without.
	if !strings.HasPrefix(links[1].URL, "https://wa.me/528112345678?text=") {
		t.Errorf("links[1].URL = %q, want a 52 prefix", links[1].URL)
	}
	for _, want := range []string{"Gabriel", "2026-08-01", "Ford", "maps.app.goo.gl"} {
		if !strings.Contains(links[0].URL, url.QueryEscape(want)) {
			t.Errorf("prefilled message missing %q", want)
		}
	}
}

func TestWhatsAppLinksSkipUnusablePhones(t *testing.T) {
	tr := stringsFor("es")
	if links := whatsappLinks(tr, billing.Appointment{ClientName: "Sin tel"}, "2026-08-01"); links != nil {
		t.Errorf("links = %+v, want none without a phone", links)
	}
	if links := whatsappLinks(tr, billing.Appointment{Phone: "s/n"}, "2026-08-01"); links != nil {
		t.Errorf("links = %+v, want none for a phone with no digits", links)
	}
}
