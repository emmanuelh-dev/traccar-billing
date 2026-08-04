package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// whatsappLink is one tap-to-chat target for a visit. A job can list a second
// contact, so a row may carry more than one.
type whatsappLink struct {
	Phone  string
	URL    string
	WebURL string
}

type appointmentRow struct {
	Appointment   billing.Appointment
	DateDisplay   string
	DateValue     string
	AmountDisplay string
	AmountValue   string
	SellerName    string
	AccountName   string
	StatusLabel   string
	Open          bool
	Late          bool
	WhatsApp      []whatsappLink
}

type appointmentsView struct {
	T        uiStrings
	Title    string
	Active   string
	Error    string
	Tenant   billing.Tenant
	Rows     []appointmentRow
	Sellers  []sellerOption
	Accounts []chargeAccount
	Today    string
	Redirect string

	Status       string
	OpenCount    int
	DoneCount    int
	PendingTotal string

	SessionExpired bool
}

var appointmentStatuses = map[string]bool{"open": true, "done": true, "canceled": true, "all": true}

// stripSpaces removes whitespace pasted into contact numbers (e.g. "55 1570
// 4658") while keeping the comma that separates a second contact.
func stripSpaces(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// waDigits keeps only the digits of a phone so wa.me accepts it, and assumes
// Mexico when the operator wrote a local 10-digit number.
func waDigits(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	number := digits.String()
	if len(number) == 10 {
		number = "52" + number
	}
	return number
}

// whatsappLinks builds the follow-up message for a visit, prefilled with what
// the customer needs to confirm: the day, the time window and the address.
func whatsappLinks(t uiStrings, a billing.Appointment, dateDisplay string) []whatsappLink {
	message := fmt.Sprintf(t.WhatsAppMsgFmt, a.ClientName, dateDisplay, a.TimeWindow)
	if a.Unit != "" {
		message += " " + fmt.Sprintf(t.WhatsAppUnitFmt, a.Unit)
	}
	if a.Address != "" {
		message += " " + fmt.Sprintf(t.WhatsAppAddrFmt, a.Address)
	}

	var links []whatsappLink
	for _, phone := range a.PhoneNumbers() {
		number := waDigits(phone)
		if number == "" {
			continue
		}
		text := url.QueryEscape(message)
		links = append(links, whatsappLink{
			Phone:  phone,
			URL:    "https://wa.me/" + number + "?text=" + text,
			WebURL: "https://web.whatsapp.com/send?phone=" + number + "&text=" + text,
		})
	}
	return links
}

func (s *Server) handleAppointments(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	status := resolveChoice(w, r, appointmentStatusCookieName, appointmentStatuses, "open")

	var filter billing.AppointmentFilter
	switch status {
	case "done":
		filter.Status = billing.AppointmentDone
	case "canceled":
		filter.Status = billing.AppointmentCanceled
	case "open":
		filter.Status = billing.AppointmentScheduled
	}

	appointments, err := s.repo.ListAppointments(r.Context(), tenant.ID, filter)
	if err != nil {
		s.logger.Error("api: list appointments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sellerOpts, sellerNames, err := s.sellerOptions(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list sellers for appointments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	accounts, err := s.chargeAccounts(r, tenant.ID)
	if err != nil {
		s.logger.Error("api: list accounts for appointments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	accountNames := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		accountNames[account.ID] = account.Name
	}

	now := s.now()
	view := appointmentsView{
		T:        t,
		Title:    t.AppointmentsPageTtl,
		Active:   "appointments",
		Error:    r.URL.Query().Get("error"),
		Tenant:   tenant,
		Sellers:  sellerOpts,
		Accounts: accounts,
		Today:    now.Format(dueDateFormat),
		Redirect: r.URL.RequestURI(),
		Status:   status,

		SessionExpired: !tenant.HasValidSession(time.Now()),
	}

	var pendingCents int64
	currency := defaultCurrency
	for _, a := range appointments {
		scheduled := a.ScheduledOn.In(s.loc)
		if a.Currency != "" {
			currency = a.Currency
		}
		switch a.Status {
		case billing.AppointmentScheduled:
			view.OpenCount++
			pendingCents += a.AmountCents
		case billing.AppointmentDone:
			view.DoneCount++
		}
		dateDisplay := scheduled.Format(dueDateFormat)
		view.Rows = append(view.Rows, appointmentRow{
			Appointment:   a,
			DateDisplay:   dateDisplay,
			DateValue:     dateDisplay,
			AmountDisplay: formatAmount(a.AmountCents, a.Currency),
			AmountValue:   centsValue(a.AmountCents),
			SellerName:    sellerNames[a.SellerID],
			AccountName:   accountNames[a.AccountID],
			StatusLabel:   t.appointmentStatusLabel(a.Status),
			Open:          a.Open(),
			// A visit still open after its day is one nobody closed, which is
			// exactly what the operator needs to chase.
			Late:     a.Open() && scheduled.Before(now.Truncate(24*time.Hour)),
			WhatsApp: whatsappLinks(t, a, dateDisplay),
		})
	}
	view.PendingTotal = formatAmount(pendingCents, currency)

	render(w, http.StatusOK, "appointments", view)
}

func (s *Server) parseAppointmentForm(r *http.Request, tenantID int64) (billing.Appointment, error) {
	var a billing.Appointment
	if err := r.ParseForm(); err != nil {
		return a, errors.New("invalid form")
	}

	a.ClientName = r.FormValue("client_name")
	if strings.TrimSpace(a.ClientName) == "" {
		return a, errors.New("client name is required")
	}

	dateRaw := r.FormValue("scheduled_on")
	if dateRaw == "" {
		return a, errors.New("date is required")
	}
	scheduled, err := time.ParseInLocation(dueDateFormat, dateRaw, s.loc)
	if err != nil {
		return a, errors.New("invalid date")
	}
	a.ScheduledOn = scheduled

	if v := r.FormValue("amount"); v != "" {
		cents, err := parseAmountCents(v)
		if err != nil {
			return a, errors.New("invalid amount")
		}
		a.AmountCents = cents
	}
	if v := r.FormValue("device_count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return a, errors.New("invalid device count")
		}
		a.DeviceCount = n
	}

	if v := r.FormValue("seller_id"); v != "" && v != "0" {
		sellerID, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return a, errors.New("invalid seller id")
		}
		if _, err := s.repo.GetSeller(r.Context(), tenantID, sellerID); errors.Is(err, billing.ErrNotFound) {
			return a, errors.New("seller not found")
		} else if err != nil {
			s.logger.Error("api: get seller for appointment", "error", err)
			return a, errors.New("internal error")
		}
		a.SellerID = sellerID
	}

	if v := r.FormValue("account_id"); v != "" && v != "0" {
		accountID, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return a, errors.New("invalid account id")
		}
		if _, err := s.repo.GetAccount(r.Context(), tenantID, accountID); errors.Is(err, billing.ErrNotFound) {
			return a, errors.New("account not found")
		} else if err != nil {
			s.logger.Error("api: get account for appointment", "error", err)
			return a, errors.New("internal error")
		}
		a.AccountID = accountID
	}

	a.Phone = stripSpaces(r.FormValue("phone"))
	a.Unit = r.FormValue("unit")
	a.Address = r.FormValue("address")
	a.TimeWindow = r.FormValue("time_window")
	a.Currency = r.FormValue("currency")
	a.Note = r.FormValue("note")

	return a, nil
}

func (s *Server) handleCreateAppointment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	appointment, err := s.parseAppointmentForm(r, tenant.ID)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	appointment.TenantID = tenant.ID
	appointment.Status = billing.AppointmentScheduled

	if _, err := s.repo.CreateAppointment(r.Context(), appointment); err != nil {
		s.logger.Error("api: create appointment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/appointments"), http.StatusSeeOther)
}

func (s *Server) handleUpdateAppointment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	appointmentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid appointment id")
		return
	}

	appointment, err := s.parseAppointmentForm(r, tenant.ID)
	if err != nil {
		redirectPageError(w, r, err.Error())
		return
	}
	appointment.ID = appointmentID
	appointment.TenantID = tenant.ID

	if _, err := s.repo.UpdateAppointment(r.Context(), appointment); errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "appointment not found")
		return
	} else if err != nil {
		s.logger.Error("api: update appointment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/appointments"), http.StatusSeeOther)
}

// handleAppointmentStatus closes, cancels or reopens a visit. It is one
// handler because the three are the same write with a different target state,
// and the form says which through a hidden field.
func (s *Server) handleAppointmentStatus(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	appointmentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid appointment id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	var status billing.AppointmentStatus
	switch r.FormValue("status") {
	case "done":
		status = billing.AppointmentDone
	case "canceled":
		status = billing.AppointmentCanceled
	case "scheduled":
		status = billing.AppointmentScheduled
	default:
		redirectPageError(w, r, "invalid status")
		return
	}

	_, err = s.repo.SetAppointmentStatus(r.Context(), tenant.ID, appointmentID, status, r.FormValue("outcome"))
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "appointment not found")
		return
	}
	if err != nil {
		s.logger.Error("api: set appointment status", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/appointments"), http.StatusSeeOther)
}

func (s *Server) handleDeleteAppointment(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())

	appointmentID, err := parseIDParam(r, "id")
	if err != nil {
		redirectPageError(w, r, "invalid appointment id")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectPageError(w, r, "invalid form")
		return
	}

	err = s.repo.DeleteAppointment(r.Context(), tenant.ID, appointmentID)
	if errors.Is(err, billing.ErrNotFound) {
		redirectPageError(w, r, "appointment not found")
		return
	}
	if err != nil {
		s.logger.Error("api: delete appointment", "error", err)
		redirectPageError(w, r, "internal error")
		return
	}
	http.Redirect(w, r, redirectTarget(r, "/appointments"), http.StatusSeeOther)
}
