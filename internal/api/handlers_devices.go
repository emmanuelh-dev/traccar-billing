package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const deviceUsageConcurrency = 8
const deviceDataCacheTTL = 2 * time.Minute

var dataQuantityPattern = regexp.MustCompile(`(?i)^([0-9]+(?:[.,][0-9]+)?)\s*(B|KB|MB|GB|TB)?$`)
var planAllowancePattern = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?)\s*(MB|GB|TB)\b`)

type deviceRow struct {
	Name             string `json:"name"`
	IMEI             string `json:"imei"`
	ProviderIMEI     string `json:"providerImei"`
	TraccarStatus    string `json:"traccarStatus"`
	ICCID            string `json:"iccid"`
	SIMStatus        string `json:"simStatus"`
	ServicePackID    string `json:"servicePackId"`
	ActivatedAt      string `json:"activatedAt"`
	RemainingTime    string `json:"remainingTime"`
	AllowedData      string `json:"allowedData"`
	UsedData         string `json:"usedData"`
	RemainingData    string `json:"remainingData"`
	UsagePercent     int    `json:"usagePercent"`
	UsagePeriod      string `json:"usagePeriod"`
	HasUsedData      bool   `json:"hasUsedData"`
	HasAllowance     bool   `json:"hasAllowance"`
	DerivedRemaining bool   `json:"derivedRemaining"`
	CanSMS           bool   `json:"canSms"`
	CanChangeStatus  bool   `json:"canChangeStatus"`
	Error            string `json:"error"`
}

type devicesView struct {
	T               uiStrings
	Title           string
	Active          string
	Tenant          billing.Tenant
	Rows            []deviceRow
	Error           string
	Warning         string
	Notice          string
	Updated         string
	CanSMS          bool
	CanChangeStatus bool

	SessionExpired bool
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	view := devicesView{
		T:              t,
		Title:          t.DevicesPageTtl,
		Active:         "devices",
		Tenant:         tenant,
		SessionExpired: !tenant.HasValidSession(s.now()),
	}

	if s.connectivity == nil {
		view.Error = t.ConnectivityNotConfigured
		render(w, http.StatusOK, "devices", view)
		return
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		view.Error = t.ConnectivityNotConfigured
		render(w, http.StatusOK, "devices", view)
		return
	}

	_, view.CanSMS = provider.(billing.SMSProvider)
	_, view.CanChangeStatus = provider.(billing.SIMStatusProvider)
	switch r.URL.Query().Get("sms") {
	case "sent":
		view.Notice = t.SMSSentNotice
	case "error":
		view.Warning = t.SMSSendError
	}

	render(w, http.StatusOK, "devices", view)
}

type devicesDataResponse struct {
	Rows            []deviceRow `json:"rows"`
	Warning         string      `json:"warning,omitempty"`
	Updated         string      `json:"updated"`
	CanSMS          bool        `json:"canSms"`
	CanChangeStatus bool        `json:"canChangeStatus"`
}

type deviceDataCacheEntry struct {
	response  devicesDataResponse
	expiresAt time.Time
}

func (s *Server) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	devices, warning, err := s.loadTraccarDevices(r.Context(), tenant, t)
	if err != nil {
		http.Error(w, t.DevicesFetchError, http.StatusBadGateway)
		return
	}
	rows := make([]deviceRow, len(devices))
	for i, device := range devices {
		rows[i] = deviceRow{Name: device.Name, IMEI: device.UniqueID, TraccarStatus: device.Status}
	}
	writeJSON(w, http.StatusOK, devicesDataResponse{
		Rows: rows, Warning: warning, Updated: s.now().Format("2006-01-02 15:04"),
	})
}

func (s *Server) handleDevicesData(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	t := stringsFor(resolveLang(w, r))
	response, err := s.loadDeviceData(r.Context(), tenant, t)
	if err != nil {
		s.logger.Error("api: load device data", "tenant_id", tenant.ID, "error", err)
		http.Error(w, t.DevicesFetchError, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) warmDeviceData(tenant billing.Tenant, t uiStrings) {
	if s.connectivity == nil {
		return
	}
	if _, ok := s.connectivity.ResolveProvider(tenant); !ok {
		return
	}
	key := deviceDataCacheKey(tenant.ID, t.Lang)
	s.deviceDataMu.Lock()
	if cached, ok := s.deviceDataCache[key]; ok && s.now().Before(cached.expiresAt) {
		s.deviceDataMu.Unlock()
		return
	}
	if _, loading := s.deviceDataLoads[key]; loading {
		s.deviceDataMu.Unlock()
		return
	}
	s.deviceDataMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := s.loadDeviceData(ctx, tenant, t); err != nil && s.logger != nil {
			s.logger.Warn("api: prewarm device data", "tenant_id", tenant.ID, "error", err)
		}
	}()
}

func (s *Server) loadDeviceData(ctx context.Context, tenant billing.Tenant, t uiStrings) (devicesDataResponse, error) {
	key := deviceDataCacheKey(tenant.ID, t.Lang)
	for {
		s.deviceDataMu.Lock()
		if cached, ok := s.deviceDataCache[key]; ok && s.now().Before(cached.expiresAt) {
			s.deviceDataMu.Unlock()
			return cached.response, nil
		}
		if loading, ok := s.deviceDataLoads[key]; ok {
			s.deviceDataMu.Unlock()
			select {
			case <-loading:
				continue
			case <-ctx.Done():
				return devicesDataResponse{}, ctx.Err()
			}
		}
		loading := make(chan struct{})
		s.deviceDataLoads[key] = loading
		s.deviceDataMu.Unlock()

		response, err := s.fetchDeviceData(ctx, tenant, t)

		s.deviceDataMu.Lock()
		if err == nil {
			s.deviceDataCache[key] = deviceDataCacheEntry{
				response: response, expiresAt: s.now().Add(deviceDataCacheTTL),
			}
		}
		close(loading)
		delete(s.deviceDataLoads, key)
		s.deviceDataMu.Unlock()
		return response, err
	}
}

func (s *Server) fetchDeviceData(ctx context.Context, tenant billing.Tenant, t uiStrings) (devicesDataResponse, error) {
	if s.connectivity == nil {
		return devicesDataResponse{}, fmt.Errorf("connectivity provider unavailable")
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		return devicesDataResponse{}, fmt.Errorf("connectivity provider unavailable")
	}
	rows, warning, err := s.loadDeviceRows(ctx, tenant, t, provider, true)
	if err != nil {
		return devicesDataResponse{}, err
	}
	_, canSMS := provider.(billing.SMSProvider)
	_, canChangeStatus := provider.(billing.SIMStatusProvider)
	return devicesDataResponse{
		Rows: rows, Warning: warning, Updated: s.now().Format("2006-01-02 15:04"),
		CanSMS: canSMS, CanChangeStatus: canChangeStatus,
	}, nil
}

func deviceDataCacheKey(tenantID int64, lang string) string {
	return strconv.FormatInt(tenantID, 10) + ":" + lang
}

func (s *Server) clearDeviceDataCache(tenantID int64) {
	prefix := strconv.FormatInt(tenantID, 10) + ":"
	s.deviceDataMu.Lock()
	defer s.deviceDataMu.Unlock()
	for key := range s.deviceDataCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.deviceDataCache, key)
		}
	}
}

func (s *Server) loadDeviceRows(ctx context.Context, tenant billing.Tenant, t uiStrings, provider billing.ConnectivityProvider, includeUsage bool) ([]deviceRow, string, error) {
	devices, warning, err := s.loadTraccarDevices(ctx, tenant, t)
	if err != nil {
		return nil, "", err
	}
	_, canSMS := provider.(billing.SMSProvider)
	_, canChangeStatus := provider.(billing.SIMStatusProvider)
	rows := make([]deviceRow, len(devices))
	for i, device := range devices {
		rows[i] = deviceRow{Name: device.Name, IMEI: device.UniqueID, TraccarStatus: device.Status}
	}

	var wg sync.WaitGroup
	limit := make(chan struct{}, deviceUsageConcurrency)
	for i, device := range devices {
		wg.Add(1)
		go func(index int, imei string) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				rows[index].Error = t.UsageUnavailable
				return
			}

			usage, err := provider.LookupDevice(ctx, imei)
			if err != nil {
				s.logger.Warn("api: enrich device with 1global", "tenant_id", tenant.ID, "imei", imei, "error", err)
				rows[index].Error = t.UsageUnavailable
				return
			}

			row := &rows[index]
			row.ProviderIMEI = usage.IMEI
			row.ICCID = usage.ICCID
			row.SIMStatus = usage.Status
			row.ServicePackID = orDash(usage.ServicePlan)
			row.ActivatedAt = usage.ActivatedAt
			row.AllowedData = displayDataQuantity(usage.AllowedData)
			row.RemainingData = displayDataQuantity(usage.RemainingData)
			row.CanSMS = canSMS
			row.CanChangeStatus = canChangeStatus

			allowed, allowedOK := parseDataBytes(usage.AllowedData)
			remaining, remainingOK := parseDataBytes(usage.RemainingData)
			if allowedOK && remainingOK {
				used := math.Max(0, allowed-remaining)
				row.UsedData = formatDataBytes(used)
				row.HasUsedData = true
				row.HasAllowance = true
				if allowed > 0 {
					row.UsagePercent = int(math.Round(math.Min(100, used/allowed*100)))
				}
			}
		}(i, device.UniqueID)
	}
	wg.Wait()
	if !includeUsage {
		return rows, warning, nil
	}

	return s.addUsageToDeviceRows(ctx, tenant, t, provider, rows, warning)
}

func (s *Server) loadTraccarDevices(ctx context.Context, tenant billing.Tenant, t uiStrings) ([]billing.TraccarDevice, string, error) {
	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		return nil, "", err
	}
	devices, err := s.client.FetchDevices(ctx, baseURL, tenant.TraccarSession())
	if err != nil {
		return nil, "", err
	}
	devices = filterRecoveryDevices(devices)
	warning := ""
	if protocolClient, ok := s.client.(billing.TraccarProtocolClient); ok {
		protocols, protocolErr := protocolClient.FetchDeviceProtocols(ctx, baseURL, tenant.TraccarSession())
		if protocolErr != nil {
			s.logger.Warn("api: fetch traccar protocols for devices page", "tenant_id", tenant.ID, "error", protocolErr)
			warning = t.DeviceProtocolFilterWarn
		} else {
			devices = filterDevicesByProtocol(devices, protocols, "osmand")
		}
	}
	sort.Slice(devices, func(i, j int) bool {
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	return devices, warning, nil
}

func (s *Server) addUsageToDeviceRows(ctx context.Context, tenant billing.Tenant, t uiStrings, provider billing.ConnectivityProvider, rows []deviceRow, warning string) ([]deviceRow, string, error) {
	iccids := make([]string, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	var earliestActivation time.Time
	for _, row := range rows {
		if row.ICCID != "" && !seen[row.ICCID] {
			seen[row.ICCID] = true
			iccids = append(iccids, row.ICCID)
		}
		if activated, err := time.Parse(time.RFC3339, row.ActivatedAt); err == nil &&
			(earliestActivation.IsZero() || activated.Before(earliestActivation)) {
			earliestActivation = activated
		}
	}
	usageRows := []billing.UsageRecord(nil)
	var err error
	lifetimeUsage := false
	if historical, ok := provider.(billing.HistoricalUsageProvider); ok && !earliestActivation.IsZero() {
		usageRows, err = historical.FetchUsageRange(ctx, iccids, earliestActivation, s.now())
		lifetimeUsage = err == nil
		if err != nil {
			s.logger.Warn("api: fetch lifetime data usage report; falling back to 30 days", "tenant_id", tenant.ID, "error", err)
			usageRows, err = provider.FetchUsage(ctx, iccids)
		}
	} else {
		usageRows, err = provider.FetchUsage(ctx, iccids)
	}
	if err != nil {
		s.logger.Warn("api: fetch 1global data usage report", "tenant_id", tenant.ID, "error", err)
		warning = t.UsageReportUnavailable
	} else {
		type usageTotal struct {
			bytes float64
			start string
			end   string
		}
		totals := make(map[string]usageTotal)
		for _, usage := range usageRows {
			value, ok := parseDataBytes(usage.TotalUsage)
			if !ok {
				continue
			}
			total := totals[usage.ICCID]
			total.bytes += value
			if total.start == "" || usage.StartDate < total.start {
				total.start = usage.StartDate
			}
			if usage.EndDate > total.end {
				total.end = usage.EndDate
			}
			totals[usage.ICCID] = total
		}
		for i := range rows {
			total, ok := totals[rows[i].ICCID]
			if !ok {
				continue
			}
			rows[i].UsedData = formatDataBytes(total.bytes)
			rows[i].HasUsedData = true
			rows[i].UsagePeriod = formatUsagePeriod(total.start, total.end)
			usageCoversSIMLife := lifetimeUsage || activatedWithin(rows[i].ActivatedAt, s.now().AddDate(0, 0, -30))
			if !rows[i].HasAllowance && usageCoversSIMLife {
				if allowance, ok := parsePlanAllowance(rows[i].ServicePackID); ok {
					rows[i].AllowedData = formatDataBytes(allowance)
					rows[i].RemainingData = formatDataBytes(math.Max(0, allowance-total.bytes))
					rows[i].HasAllowance = true
					rows[i].DerivedRemaining = true
					if allowance > 0 {
						rows[i].UsagePercent = int(math.Round(math.Min(100, total.bytes/allowance*100)))
					}
				}
			}
		}
	}
	return rows, warning, nil
}

func parsePlanAllowance(plan string) (float64, bool) {
	match := planAllowancePattern.FindStringSubmatch(plan)
	if match == nil {
		return 0, false
	}
	return parseDataBytes(match[1] + " " + match[2])
}

func activatedWithin(raw string, start time.Time) bool {
	activated, err := time.Parse(time.RFC3339, raw)
	return err == nil && !activated.Before(start)
}

func filterRecoveryDevices(devices []billing.TraccarDevice) []billing.TraccarDevice {
	filtered := make([]billing.TraccarDevice, 0, len(devices))
	for _, device := range devices {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(device.Name)), "recovery-") {
			continue
		}
		filtered = append(filtered, device)
	}
	return filtered
}

func filterDevicesByProtocol(devices []billing.TraccarDevice, protocols map[int64]string, excluded string) []billing.TraccarDevice {
	filtered := make([]billing.TraccarDevice, 0, len(devices))
	for _, device := range devices {
		if strings.EqualFold(strings.TrimSpace(protocols[device.ID]), excluded) {
			continue
		}
		filtered = append(filtered, device)
	}
	return filtered
}

func (s *Server) handleDeviceSMSHistory(w http.ResponseWriter, r *http.Request) {
	smsProvider, sim, ok := s.resolveTenantSMSDevice(w, r)
	if !ok {
		return
	}
	messages, err := smsProvider.FetchSMSHistory(r.Context(), sim.ICCID, 20)
	if err != nil {
		s.logger.Warn("api: fetch sms history", "iccid", sim.ICCID, "error", err)
		http.Error(w, "sms history unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleSendDeviceSMS(w http.ResponseWriter, r *http.Request) {
	smsProvider, sim, ok := s.resolveTenantSMSDevice(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/devices?sms=error", http.StatusSeeOther)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" || len([]rune(text)) > 1600 {
		http.Redirect(w, r, "/devices?sms=error", http.StatusSeeOther)
		return
	}
	if _, err := smsProvider.SendSMS(r.Context(), sim.ICCID, text); err != nil {
		s.logger.Error("api: send device sms", "iccid", sim.ICCID, "error", err)
		http.Redirect(w, r, "/devices?sms=error", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/devices?sms=sent", http.StatusSeeOther)
}

// resolveTenantSMSDevice checks the IMEI against the tenant's own Traccar
// devices before resolving its ICCID. This prevents a tenant from editing the
// URL to send messages to an unrelated SIM in the provider account.
func (s *Server) resolveTenantSMSDevice(w http.ResponseWriter, r *http.Request) (billing.SMSProvider, billing.SIMInfo, bool) {
	tenant := tenantFromContext(r.Context())
	if s.connectivity == nil {
		http.Error(w, "connectivity provider unavailable", http.StatusServiceUnavailable)
		return nil, billing.SIMInfo{}, false
	}
	provider, ok := s.connectivity.ResolveProvider(tenant)
	if !ok {
		http.Error(w, "connectivity provider unavailable", http.StatusServiceUnavailable)
		return nil, billing.SIMInfo{}, false
	}
	smsProvider, ok := provider.(billing.SMSProvider)
	if !ok {
		http.Error(w, "sms unavailable", http.StatusNotImplemented)
		return nil, billing.SIMInfo{}, false
	}
	baseURL, err := url.Parse(tenant.BaseURL)
	if err != nil {
		http.Error(w, "invalid tenant url", http.StatusInternalServerError)
		return nil, billing.SIMInfo{}, false
	}
	imei := strings.TrimSpace(chi.URLParam(r, "imei"))
	devices, err := s.client.FetchDevices(r.Context(), baseURL, tenant.TraccarSession())
	if err != nil {
		http.Error(w, "devices unavailable", http.StatusBadGateway)
		return nil, billing.SIMInfo{}, false
	}
	owned := false
	for _, device := range devices {
		if device.UniqueID == imei {
			owned = true
			break
		}
	}
	if !owned {
		http.Error(w, "device not found", http.StatusNotFound)
		return nil, billing.SIMInfo{}, false
	}
	sim, err := provider.LookupDevice(r.Context(), imei)
	if err != nil || sim.ICCID == "" {
		http.Error(w, "sim unavailable", http.StatusBadGateway)
		return nil, billing.SIMInfo{}, false
	}
	return smsProvider, sim, true
}

func formatUsagePeriod(start, end string) string {
	if start == "" && end == "" {
		return "—"
	}
	if len(start) >= 10 {
		start = start[:10]
	}
	if len(end) >= 10 {
		end = end[:10]
	}
	return start + " – " + end
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func parseDataBytes(raw string) (float64, bool) {
	match := dataQuantityPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.Replace(match[1], ",", ".", 1), 64)
	if err != nil {
		return 0, false
	}
	multiplier := float64(1024 * 1024) // A bare API value is expressed in MB.
	switch strings.ToUpper(match[2]) {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "", "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}
	return value * multiplier, true
}

func displayDataQuantity(raw string) string {
	if value, ok := parseDataBytes(raw); ok {
		return formatDataBytes(value)
	}
	if strings.TrimSpace(raw) == "" {
		return "—"
	}
	return raw
}

func formatDataBytes(bytes float64) string {
	const (
		mb = 1024 * 1024
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", bytes/tb)
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", bytes/gb)
	default:
		return fmt.Sprintf("%.2f MB", bytes/mb)
	}
}
