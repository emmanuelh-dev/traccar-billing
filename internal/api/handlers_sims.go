package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type simsView struct {
	T              uiStrings
	Title          string
	Active         string
	Tenant         billing.Tenant
	Error          string
	SessionExpired bool
}

func (s *Server) handleSIMs(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	view := simsView{
		T: t, Title: t.SIMsPageTtl, Active: "sims", Tenant: tenant,
		SessionExpired: !tenant.HasValidSession(s.now()),
	}
	if s.connectivity == nil {
		view.Error = t.ConnectivityNotConfigured
	} else if _, ok := s.connectivity.ResolveProvider(tenant); !ok {
		view.Error = t.ConnectivityNotConfigured
	}
	render(w, http.StatusOK, "sims", view)
}

func (s *Server) handleSIMsData(w http.ResponseWriter, r *http.Request) {
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
	inventory, ok := provider.(billing.SIMInventoryProvider)
	if !ok {
		http.Error(w, t.SIMInventoryUnavailable, http.StatusNotImplemented)
		return
	}
	sims, err := inventory.ListSIMs(r.Context())
	if err != nil {
		s.logger.Warn("api: list provider sims", "tenant_id", tenant.ID, "error", err)
		http.Error(w, t.SIMInventoryUnavailable, http.StatusBadGateway)
		return
	}
	_, canChangeStatus := provider.(billing.SIMStatusProvider)
	rows := make([]deviceRow, len(sims))
	for i, sim := range sims {
		name := strings.TrimSpace(sim.Label)
		if name == "" {
			name = strings.TrimSpace(sim.Description)
		}
		if name == "" {
			name = "SIM"
		}
		rows[i] = deviceRow{
			Name: name, IMEI: sim.IMEI, ProviderIMEI: sim.IMEI, ICCID: sim.ICCID,
			SIMStatus: sim.Status, ServicePackID: orDash(sim.ServicePlan),
			ActivatedAt: sim.ActivatedAt, CanChangeStatus: canChangeStatus,
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	rows, warning, err := s.addUsageToDeviceRows(r.Context(), tenant, t, provider, rows, "")
	if err != nil {
		http.Error(w, t.SIMInventoryUnavailable, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, devicesDataResponse{
		Rows: rows, Warning: warning, Updated: s.now().Format("2006-01-02 15:04"),
	})
}

type simStatusRequest struct {
	ICCID  string `json:"iccid"`
	Status string `json:"status"`
}

func (s *Server) handleSIMStatus(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))

	var request simStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, t.SIMStatusInvalid)
		return
	}
	iccid := strings.TrimSpace(request.ICCID)
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	if iccid == "" || (status != "ACTIVE" && status != "SUSPENDED") {
		// RETIRED and the rest are deliberately not exposed because they are
		// irreversible.
		writeJSONError(w, http.StatusBadRequest, t.SIMStatusInvalid)
		return
	}

	if s.connectivity == nil {
		writeJSONError(w, http.StatusServiceUnavailable, t.ConnectivityNotConfigured)
		return
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, t.ConnectivityNotConfigured)
		return
	}
	statusProvider, ok := provider.(billing.SIMStatusProvider)
	if !ok {
		writeJSONError(w, http.StatusNotImplemented, t.SIMStatusUnavailable)
		return
	}

	// Never act on an ICCID outside the tenant's own account, the same spirit
	// as resolveTenantSMSDevice.
	inventory, ok := provider.(billing.SIMInventoryProvider)
	if !ok {
		writeJSONError(w, http.StatusNotImplemented, t.SIMStatusUnavailable)
		return
	}
	sims, err := inventory.ListSIMs(r.Context())
	if err != nil {
		s.logger.Warn("api: list provider sims", "tenant_id", tenant.ID, "error", err)
		writeJSONError(w, http.StatusBadGateway, t.SIMInventoryUnavailable)
		return
	}
	owned := false
	for _, sim := range sims {
		if sim.ICCID == iccid {
			owned = true
			break
		}
	}
	if !owned {
		writeJSONError(w, http.StatusNotFound, t.SIMStatusInvalid)
		return
	}

	if err := statusProvider.ChangeSIMStatus(r.Context(), []string{iccid}, status); err != nil {
		s.logger.Error("api: change sim status", "tenant_id", tenant.ID, "iccid", iccid, "error", err)
		writeJSONError(w, http.StatusBadGateway, t.SIMStatusFailed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"iccid": iccid, "status": status})
}
