package api

import (
	"net/http"
	"sort"
	"strings"
	"sync"
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
	if _, ok := provider.(billing.SIMInventoryProvider); !ok {
		http.Error(w, t.SMSHistoryUnavailable, http.StatusNotImplemented)
		return
	}

	// ListSIMs and FetchSMSHistory each take seconds and are independent, so
	// run them concurrently instead of paying for both in series.
	var sims []billing.SIMInfo
	var simsErr error
	var messages []billing.SMSMessage
	var msgErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sims, simsErr = s.cachedSIMs(r.Context(), tenant, t)
	}()
	go func() {
		defer wg.Done()
		messages, msgErr = smsProvider.FetchSMSHistory(r.Context(), "", 100)
	}()
	wg.Wait()

	if msgErr != nil {
		s.logger.Warn("api: fetch tenant sim history", "tenant_id", tenant.ID, "error", msgErr)
		http.Error(w, t.SMSHistoryUnavailable, http.StatusBadGateway)
		return
	}

	// Device names are a nicety for display only; a failure here should not
	// take down the whole history page.
	inventoryOK := simsErr == nil
	if simsErr != nil {
		s.logger.Warn("api: list provider sims for sim history", "tenant_id", tenant.ID, "error", simsErr)
	}
	devices, _, devicesErr := s.loadTraccarDevices(r.Context(), tenant, t)
	if devicesErr != nil {
		s.logger.Warn("api: load traccar devices for sim history", "tenant_id", tenant.ID, "error", devicesErr)
		devices = nil
	}

	// Index Traccar devices by UniqueID, and also by its first 14 characters
	// when it is a 15-digit IMEI: 1GLOBAL stores the 14-digit TAC+serial while
	// Traccar stores the full 15-digit IMEI with its Luhn check digit (see
	// LookupDevice in internal/truphone/client.go).
	byUniqueID := make(map[string]billing.TraccarDevice, len(devices))
	for _, device := range devices {
		byUniqueID[device.UniqueID] = device
		if len(device.UniqueID) == 15 {
			byUniqueID[device.UniqueID[:14]] = device
		}
	}

	names := make(map[string]string, len(sims))
	iccidSet := make(map[string]bool, len(sims))
	for _, sim := range sims {
		iccidSet[sim.ICCID] = true
		name := ""
		if device, ok := byUniqueID[sim.IMEI]; ok {
			name = device.Name
		} else {
			name = strings.TrimSpace(sim.Label)
		}
		names[sim.ICCID] = name
	}

	type rowWithTime struct {
		row simHistoryRow
		at  time.Time
	}
	dated := make([]rowWithTime, 0, len(messages))
	for _, message := range messages {
		// A SIM belongs to the tenant when it appears in the tenant's own
		// provider inventory. If the inventory failed to load, skip the
		// filter rather than dropping every row.
		if inventoryOK && !iccidSet[message.ICCID] {
			continue
		}
		row := simHistoryRow{
			ID: message.ID, DateSubmitted: message.DateSubmitted, Device: names[message.ICCID],
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
