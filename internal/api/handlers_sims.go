package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// simSnapshotTTL: the provider inventory changes only when the operator buys
// or retires SIMs, so a day-old snapshot is fine and saves ~8s per page load.
// The refresh button on /sims covers the impatient case.
const simSnapshotTTL = 24 * time.Hour

// simSnapshot is stored as JSON and must stay language-neutral: anything
// localized (warnings, capability flags) is recomputed at serve time.
type simSnapshot struct {
	SIMs         []billing.SIMInfo `json:"sims"`
	Rows         []deviceRow       `json:"rows"`
	UsageWarning bool              `json:"usageWarning"`
}

// loadSIMSnapshot returns the tenant's persisted inventory snapshot,
// refreshing it when missing, stale, or forced. Concurrent callers for the
// same tenant join a single in-flight refresh instead of each paying the ~8s
// provider round trip.
func (s *Server) loadSIMSnapshot(ctx context.Context, tenant billing.Tenant, t uiStrings, force bool) (simSnapshot, time.Time, error) {
	for {
		if !force {
			if payload, refreshedAt, found, err := s.repo.GetSIMInventoryCache(ctx, tenant.ID); err == nil && found {
				if s.now().Sub(refreshedAt) < simSnapshotTTL {
					var snapshot simSnapshot
					if err := json.Unmarshal([]byte(payload), &snapshot); err == nil {
						return snapshot, refreshedAt, nil
					}
					s.logger.Warn("api: unmarshal sim inventory snapshot", "tenant_id", tenant.ID)
				}
			}
		}

		s.simCacheMu.Lock()
		if loading, ok := s.simLoads[tenant.ID]; ok {
			s.simCacheMu.Unlock()
			select {
			case <-loading:
				force = false // a fresh snapshot just landed; re-read it instead of forcing another
				continue
			case <-ctx.Done():
				return simSnapshot{}, time.Time{}, ctx.Err()
			}
		}
		loading := make(chan struct{})
		s.simLoads[tenant.ID] = loading
		s.simCacheMu.Unlock()

		snapshot, refreshedAt, err := s.refreshSIMSnapshot(ctx, tenant, t)

		s.simCacheMu.Lock()
		close(loading)
		delete(s.simLoads, tenant.ID)
		s.simCacheMu.Unlock()

		return snapshot, refreshedAt, err
	}
}

// refreshSIMSnapshot fetches fresh data from the provider and persists it. It
// does not itself de-duplicate concurrent callers; use loadSIMSnapshot for that.
func (s *Server) refreshSIMSnapshot(ctx context.Context, tenant billing.Tenant, t uiStrings) (simSnapshot, time.Time, error) {
	if s.connectivity == nil {
		return simSnapshot{}, time.Time{}, fmt.Errorf("connectivity provider unavailable")
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		return simSnapshot{}, time.Time{}, fmt.Errorf("connectivity provider unavailable")
	}
	inventory, ok := provider.(billing.SIMInventoryProvider)
	if !ok {
		return simSnapshot{}, time.Time{}, fmt.Errorf("sim inventory unavailable")
	}

	sims, err := inventory.ListSIMs(ctx)
	if err != nil {
		return simSnapshot{}, time.Time{}, err
	}
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
			ActivatedAt: sim.ActivatedAt,
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	rows, warning, err := s.addUsageToDeviceRows(ctx, tenant, t, provider, rows, "")
	if err != nil {
		return simSnapshot{}, time.Time{}, err
	}

	refreshedAt := s.now()
	snapshot := simSnapshot{SIMs: sims, Rows: rows, UsageWarning: warning != ""}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		s.logger.Warn("api: marshal sim inventory snapshot", "tenant_id", tenant.ID, "error", err)
		return snapshot, refreshedAt, nil
	}
	if err := s.repo.SaveSIMInventoryCache(ctx, tenant.ID, string(payload), refreshedAt); err != nil {
		s.logger.Warn("api: save sim inventory snapshot", "tenant_id", tenant.ID, "error", err)
	}
	return snapshot, refreshedAt, nil
}

// cachedSIMs returns the tenant's SIM list from the persisted snapshot,
// refreshing it first when stale.
func (s *Server) cachedSIMs(ctx context.Context, tenant billing.Tenant, t uiStrings) ([]billing.SIMInfo, error) {
	snapshot, _, err := s.loadSIMSnapshot(ctx, tenant, t, false)
	if err != nil {
		return nil, err
	}
	return snapshot.SIMs, nil
}

func (s *Server) clearSIMSnapshot(ctx context.Context, tenantID int64) {
	if err := s.repo.DeleteSIMInventoryCache(ctx, tenantID); err != nil {
		s.logger.Warn("api: delete sim inventory snapshot", "tenant_id", tenantID, "error", err)
	}
}

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
	if _, ok := provider.(billing.SIMInventoryProvider); !ok {
		http.Error(w, t.SIMInventoryUnavailable, http.StatusNotImplemented)
		return
	}
	snapshot, refreshedAt, err := s.loadSIMSnapshot(r.Context(), tenant, t, false)
	if err != nil {
		s.logger.Warn("api: load sim inventory snapshot", "tenant_id", tenant.ID, "error", err)
		http.Error(w, t.SIMInventoryUnavailable, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, s.simsDataResponse(snapshot, refreshedAt, provider, t))
}

// simsDataResponse turns a persisted snapshot into the response shape the
// page expects, applying the serve-time flags that must not be cached
// (capability flags and localized warnings).
func (s *Server) simsDataResponse(snapshot simSnapshot, refreshedAt time.Time, provider billing.ConnectivityProvider, t uiStrings) devicesDataResponse {
	_, canChangeStatus := provider.(billing.SIMStatusProvider)
	rows := make([]deviceRow, len(snapshot.Rows))
	copy(rows, snapshot.Rows)
	for i := range rows {
		rows[i].CanChangeStatus = canChangeStatus
	}
	warning := ""
	if snapshot.UsageWarning {
		warning = t.UsageReportUnavailable
	}
	return devicesDataResponse{
		// refreshedAt is stored in UTC; the page shows the tenant's own clock.
		Rows: rows, Warning: warning, Updated: refreshedAt.In(s.loc).Format("2006-01-02 15:04"),
	}
}

func (s *Server) handleSIMsRefresh(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	if s.connectivity == nil {
		writeJSONError(w, http.StatusServiceUnavailable, t.ConnectivityNotConfigured)
		return
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, t.ConnectivityNotConfigured)
		return
	}
	if _, ok := provider.(billing.SIMInventoryProvider); !ok {
		writeJSONError(w, http.StatusNotImplemented, t.SIMInventoryUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	snapshot, refreshedAt, err := s.loadSIMSnapshot(ctx, tenant, t, true)
	if err != nil {
		s.logger.Warn("api: refresh sim inventory", "tenant_id", tenant.ID, "error", err)
		writeJSONError(w, http.StatusBadGateway, t.SIMRefreshFailed)
		return
	}
	writeJSON(w, http.StatusOK, s.simsDataResponse(snapshot, refreshedAt, provider, t))
}

// RefreshSIMInventory is called by the scheduler once a day per tenant to
// keep the first /sims load of the day off the critical path.
func (s *Server) RefreshSIMInventory(ctx context.Context, tenant billing.Tenant) error {
	if s.connectivity == nil {
		return nil
	}
	if _, ok := s.connectivity.ResolveProvider(tenant); !ok {
		return nil
	}
	// loadSIMSnapshot(force=false) is itself a no-op when the stored snapshot
	// is still within simSnapshotTTL, so the 24h decision lives there.
	_, _, err := s.loadSIMSnapshot(ctx, tenant, stringsFor("es"), false)
	return err
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
	if _, ok := provider.(billing.SIMInventoryProvider); !ok {
		writeJSONError(w, http.StatusNotImplemented, t.SIMStatusUnavailable)
		return
	}
	snapshot, refreshedAt, err := s.loadSIMSnapshot(r.Context(), tenant, t, false)
	if err != nil {
		s.logger.Warn("api: list provider sims", "tenant_id", tenant.ID, "error", err)
		writeJSONError(w, http.StatusBadGateway, t.SIMInventoryUnavailable)
		return
	}
	owned := false
	for _, sim := range snapshot.SIMs {
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

	// The status change already succeeded; patch the snapshot in place rather
	// than dropping it, so we don't throw away the ~8s of work that produced
	// it. Keep the original refreshedAt so the 24h clock is not reset.
	for i := range snapshot.SIMs {
		if snapshot.SIMs[i].ICCID == iccid {
			snapshot.SIMs[i].Status = status
		}
	}
	for i := range snapshot.Rows {
		if snapshot.Rows[i].ICCID == iccid {
			snapshot.Rows[i].SIMStatus = status
		}
	}
	if payload, err := json.Marshal(snapshot); err != nil {
		s.logger.Warn("api: marshal sim inventory snapshot", "tenant_id", tenant.ID, "error", err)
	} else if err := s.repo.SaveSIMInventoryCache(r.Context(), tenant.ID, string(payload), refreshedAt); err != nil {
		s.logger.Warn("api: save sim inventory snapshot", "tenant_id", tenant.ID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"iccid": iccid, "status": status})
}
