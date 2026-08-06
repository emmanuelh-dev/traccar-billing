package api

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

type noCallConnectivityProvider struct{}

func (noCallConnectivityProvider) ID() string { return "test" }
func (noCallConnectivityProvider) LookupDevice(context.Context, string) (billing.SIMInfo, error) {
	panic("LookupDevice must not run during initial render")
}
func (noCallConnectivityProvider) FetchUsage(context.Context, []string) ([]billing.UsageRecord, error) {
	panic("FetchUsage must not run during initial render")
}

type fixedConnectivityResolver struct{ provider billing.ConnectivityProvider }

func (r fixedConnectivityResolver) ResolveProvider(billing.Tenant) (billing.ConnectivityProvider, bool) {
	return r.provider, true
}

func TestParseAndFormatDataQuantity(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "512", want: "512.00 MB"},
		{raw: "1024 MB", want: "1.00 GB"},
		{raw: "1.5 GB", want: "1.50 GB"},
		{raw: "500 KB", want: "0.49 MB"},
	}
	for _, tt := range tests {
		value, ok := parseDataBytes(tt.raw)
		if !ok {
			t.Fatalf("parseDataBytes(%q) failed", tt.raw)
		}
		if got := formatDataBytes(value); got != tt.want {
			t.Errorf("formatDataBytes(parseDataBytes(%q)) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestRenderDevices(t *testing.T) {
	view := devicesView{
		T:       stringsFor("es"),
		Title:   "Consumo de dispositivos",
		Active:  "devices",
		Tenant:  billing.Tenant{Name: "Flota"},
		Updated: "2026-08-05 12:00",
		Rows: []deviceRow{{
			Name:          "Camión 24",
			IMEI:          "123456789012345",
			TraccarStatus: "online",
			ICCID:         "8944470000000000001",
			SIMStatus:     "ACTIVE",
			ServicePackID: "PLAN-1GB",
			UsedData:      "256.00 MB",
			HasUsedData:   true,
			UsagePeriod:   "2026-07-07 – 2026-08-05",
		}},
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "devices", view); err != nil {
		t.Fatalf("render devices: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`href="/devices" class="active"`,
		`fetch("/devices/data"`,
		"Cargando dispositivos",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("devices output missing %q", want)
		}
	}
}

func TestParsePlanAllowance(t *testing.T) {
	bytes, ok := parsePlanAllowance("Teltonika Global - 500MB - 5Years")
	if !ok || formatDataBytes(bytes) != "500.00 MB" {
		t.Fatalf("parsePlanAllowance() = %v, %v", bytes, ok)
	}
}

func TestDevicesInitialRenderDoesNotCallExternalAPIs(t *testing.T) {
	server := &Server{
		connectivity: fixedConnectivityResolver{provider: noCallConnectivityProvider{}},
		loc:          time.UTC,
	}
	request := httptest.NewRequest("GET", "/devices", nil)
	request = request.WithContext(context.WithValue(
		request.Context(), tenantContextKey, billing.Tenant{ID: 7, Name: "Flota"},
	))
	response := httptest.NewRecorder()

	server.handleDevices(response, request)

	if response.Code != 200 || !strings.Contains(response.Body.String(), `fetch("/devices/data"`) {
		t.Fatalf("initial response = %d %s", response.Code, response.Body.String())
	}
}

func TestFilterRecoveryAndOsmandDevices(t *testing.T) {
	devices := []billing.TraccarDevice{
		{ID: 1, Name: "recovery-353994714037790"},
		{ID: 2, Name: "Unidad Osmand"},
		{ID: 3, Name: "RETRO"},
	}
	devices = filterRecoveryDevices(devices)
	devices = filterDevicesByProtocol(devices, map[int64]string{2: "osmand", 3: "teltonika"}, "osmand")
	if len(devices) != 1 || devices[0].Name != "RETRO" {
		t.Fatalf("filtered devices = %+v", devices)
	}
}
