package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type simHistoryView struct {
	T              uiStrings
	Title          string
	Active         string
	Tenant         billing.Tenant
	Error          string
	SessionExpired bool
}

type simHistoryRow struct {
	ID             string `json:"id"`
	DateSubmitted  string `json:"dateSubmitted"`
	DateLocal      string `json:"dateLocal"`
	Device         string `json:"device"`
	ICCID          string `json:"iccid"`
	Content        string `json:"content"`
	DeliveryStatus string `json:"deliveryStatus"`
	Direction      string `json:"direction"`
}

func (s *Server) handleSIMHistory(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	view := simHistoryView{
		T: t, Title: t.SIMHistoryPageTtl, Active: "sim-history", Tenant: tenant,
		SessionExpired: !tenant.HasValidSession(s.now()),
	}
	if s.connectivity == nil {
		view.Error = t.ConnectivityNotConfigured
	}
	render(w, http.StatusOK, "sim_history", view)
}

func (s *Server) handleSIMHistoryData(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	if s.connectivity == nil {
		http.Error(w, t.ConnectivityNotConfigured, http.StatusServiceUnavailable)
		return
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		http.Error(w, t.ConnectivityNotConfigured, http.StatusServiceUnavailable)
		return
	}
	smsProvider, ok := provider.(billing.SMSProvider)
	if !ok {
		http.Error(w, t.SMSHistoryUnavailable, http.StatusNotImplemented)
		return
	}

	deviceData, err := s.loadDeviceData(r.Context(), tenant, t)
	if err != nil {
		http.Error(w, t.DevicesFetchError, http.StatusBadGateway)
		return
	}
	owned := make(map[string]string)
	for _, device := range deviceData.Rows {
		if device.ICCID != "" {
			owned[device.ICCID] = device.Name
		}
	}

	messages, err := smsProvider.FetchSMSHistory(r.Context(), "", 100)
	if err != nil {
		s.logger.Warn("api: fetch tenant sim history", "tenant_id", tenant.ID, "error", err)
		http.Error(w, t.SMSHistoryUnavailable, http.StatusBadGateway)
		return
	}
	type rowWithTime struct {
		row simHistoryRow
		at  time.Time
	}
	dated := make([]rowWithTime, 0, len(messages))
	for _, message := range messages {
		device, belongsToTenant := owned[message.ICCID]
		if !belongsToTenant {
			continue
		}
		row := simHistoryRow{
			ID: message.ID, DateSubmitted: message.DateSubmitted, Device: device,
			ICCID: message.ICCID, Content: message.Content,
			DeliveryStatus: message.DeliveryStatus, Direction: message.Direction,
		}
		var at time.Time
		if parsed, err := time.Parse(time.RFC3339, message.DateSubmitted); err == nil {
			row.DateLocal = parsed.In(s.loc).Format("2006-01-02 15:04")
			at = parsed
		}
		dated = append(dated, rowWithTime{row: row, at: at})
	}
	// Rows whose date failed to parse (zero time) sort last, not by provider order.
	sort.SliceStable(dated, func(i, j int) bool { return dated[i].at.After(dated[j].at) })
	rows := make([]simHistoryRow, len(dated))
	for i, d := range dated {
		rows[i] = d.row
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows": rows, "updated": s.now().Format("2006-01-02 15:04"),
	})
}
