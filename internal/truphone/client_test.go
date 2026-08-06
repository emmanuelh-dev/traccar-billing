package truphone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestFetchDeviceUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token provider-secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v2.2/devices/123456789012345/":
			_ = json.NewEncoder(w).Encode(deviceDTO{IMEI: "123456789012345", ICCID: "8944470000000000001"})
		case "/api/v2.0/sims/8944470000000000001/":
			var dto subscriptionDTO
			dto.Subscription.ServicePackID = "PLAN-1GB"
			dto.Subscription.SubscriptionStatus = "ACTIVE"
			_ = json.NewEncoder(w).Encode(dto)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	usage, err := client.LookupDevice(context.Background(), "123456789012345")
	if err != nil {
		t.Fatalf("LookupDevice() error = %v", err)
	}
	if usage.ICCID != "8944470000000000001" || usage.ServicePlan != "PLAN-1GB" {
		t.Errorf("LookupDevice() = %+v", usage)
	}
}

func TestListSIMs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2.0/sims/" || r.URL.Query().Get("per_page") != "500" {
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		var sim simDTO
		sim.IMEI = "12345678901234"
		sim.ICCID = "8944470000000000001"
		sim.Label = "Machine 1"
		sim.Subscription.ServicePackID = "500MB"
		sim.Subscription.SubscriptionStatus = "ACTIVE"
		sim.FirstActivationDate = "2025-08-01T00:00:00Z"
		_ = json.NewEncoder(w).Encode([]simDTO{sim})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	sims, err := client.ListSIMs(context.Background())
	if err != nil {
		t.Fatalf("ListSIMs() error = %v", err)
	}
	if len(sims) != 1 || sims[0].Label != "Machine 1" || sims[0].ServicePlan != "500MB" {
		t.Fatalf("ListSIMs() = %+v", sims)
	}
	if sims[0].ActivatedAt != "2025-08-01T00:00:00Z" {
		t.Errorf("ActivatedAt = %q, want top-level firstActivationDate", sims[0].ActivatedAt)
	}
}

// The live API sends the IMEI quoted on /devices but bare on /sims, which used
// to abort the whole listing with a decode error.
func TestListSIMsAcceptsNumericIMEI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"imei":86469506011356,"iccid":"8944474400002600658","label":"Solar",` +
			`"subscription":{"servicePackId":"500MB","subscriptionStatus":"ACTIVE"},` +
			`"dates":{"firstActivationDate":"2025-04-04T23:17:40+00:00"}}]`))
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	sims, err := client.ListSIMs(context.Background())
	if err != nil {
		t.Fatalf("ListSIMs() error = %v", err)
	}
	if len(sims) != 1 || sims[0].IMEI != "86469506011356" {
		t.Fatalf("ListSIMs() = %+v", sims)
	}
	if sims[0].ActivatedAt != "2025-04-04T23:17:40+00:00" {
		t.Errorf("ActivatedAt = %q", sims[0].ActivatedAt)
	}
}

func TestListSIMsPaginatesUntilShortPage(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2.0/sims/" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		count := simListPageSize
		if page == "2" {
			count = 10
		}
		sims := make([]simDTO, count)
		for i := range sims {
			sims[i].ICCID = page + "-" + strconv.Itoa(i)
		}
		_ = json.NewEncoder(w).Encode(sims)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	sims, err := client.ListSIMs(context.Background())
	if err != nil {
		t.Fatalf("ListSIMs() error = %v", err)
	}
	if len(sims) != simListPageSize+10 {
		t.Fatalf("ListSIMs() returned %d sims, want %d", len(sims), simListPageSize+10)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Fatalf("pages requested = %v", pages)
	}
}

func TestListSIMsFallsBackToV22(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2.0/sims/":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2.2/sims/":
			var sim simDTO
			sim.ICCID = "8944470000000000002"
			sim.Dates.FirstActivationDate = "2024-01-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode([]simDTO{sim})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	sims, err := client.ListSIMs(context.Background())
	if err != nil {
		t.Fatalf("ListSIMs() error = %v", err)
	}
	if len(sims) != 1 || sims[0].ICCID != "8944470000000000002" || sims[0].ActivatedAt != "2024-01-01T00:00:00Z" {
		t.Fatalf("ListSIMs() = %+v", sims)
	}
}

func TestChangeSIMStatus(t *testing.T) {
	var idempotencyKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2.0/sims/change_status/" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		idempotencyKey = r.Header.Get("Idempotency-Key")
		var request changeStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.ICCIDs) != 1 || request.ICCIDs[0] != "8944470000000000001" || request.Status != "ACTIVE" {
			t.Errorf("change status request = %+v", request)
		}
		_ = json.NewEncoder(w).Encode(changeStatusResponse{Detail: "OK"})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	if err := client.ChangeSIMStatus(context.Background(), []string{"8944470000000000001"}, "ACTIVE"); err != nil {
		t.Fatalf("ChangeSIMStatus() error = %v", err)
	}
	if idempotencyKey == "" {
		t.Error("expected an Idempotency-Key header")
	}
}

func TestFetchDataUsage(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2.0/reports/generate/":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(reportResponse{DownloadURL: srv.URL + "/api/v2.0/reports/download/report-1/"})
		case "/api/v2.0/reports/download/report-1/":
			_ = json.NewEncoder(w).Encode([]reportRow{{
				ICCID: "8944470000000000001", TotalUsage: "12.5 MB",
				StartDate: "2026-08-01", EndDate: "2026-08-02",
			}})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	rows, err := client.FetchUsage(context.Background(), []string{"8944470000000000001"})
	if err != nil {
		t.Fatalf("FetchUsage() error = %v", err)
	}
	if len(rows) != 1 || rows[0].TotalUsage != "12.5 MB" {
		t.Errorf("FetchUsage() = %+v", rows)
	}
}

func TestFetchDataUsageRangeUsesMonthlyGranularity(t *testing.T) {
	var request reportRequest
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2.0/reports/generate/":
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(reportResponse{DownloadURL: srv.URL + "/api/v2.0/reports/download/report-range/"})
		case "/api/v2.0/reports/download/report-range/":
			_ = json.NewEncoder(w).Encode([]reportRow{})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	start := time.Date(2021, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 5, 23, 59, 59, 0, time.UTC)
	if _, err := client.FetchUsageRange(context.Background(), []string{"8944470000000000001"}, start, end); err != nil {
		t.Fatalf("FetchUsageRange() error = %v", err)
	}
	if request.Granularity != "Month" || request.Period != "" ||
		request.StartDate != "2021-08-01 00:00:00" || request.EndDate != "2026-08-05 23:59:59" {
		t.Errorf("range request = %+v", request)
	}
}

func TestNormalizeReportUsageTreatsBareValuesAsKilobytes(t *testing.T) {
	if got := normalizeReportUsage("22608"); got != "22608 KB" {
		t.Errorf("normalizeReportUsage() = %q", got)
	}
	if got := normalizeReportUsage("12.5 MB"); got != "12.5 MB" {
		t.Errorf("normalizeReportUsage() changed explicit unit: %q", got)
	}
}

func TestFetchDeviceUsageNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	_, err := client.LookupDevice(context.Background(), "123")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LookupDevice() error = %v, want ErrNotFound", err)
	}
}

func TestLookupDeviceFallsBackToFourteenDigitIMEI(t *testing.T) {
	const fullIMEI = "863829076755329"
	const providerIMEI = "86382907675532"
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2.2/devices/" + fullIMEI + "/":
			_ = json.NewEncoder(w).Encode(deviceDTO{IMEI: fullIMEI})
		case "/api/v2.2/devices/" + providerIMEI + "/":
			_ = json.NewEncoder(w).Encode(deviceDTO{IMEI: providerIMEI, ICCID: "8944470000000000001"})
		case "/api/v2.0/sims/8944470000000000001/":
			_ = json.NewEncoder(w).Encode(subscriptionDTO{})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	sim, err := client.LookupDevice(context.Background(), fullIMEI)
	if err != nil {
		t.Fatalf("LookupDevice() error = %v", err)
	}
	if sim.IMEI != providerIMEI || sim.ICCID != "8944470000000000001" {
		t.Errorf("LookupDevice() = %+v", sim)
	}
	if len(paths) != 3 {
		t.Errorf("request paths = %v", paths)
	}
}

func TestLookupDeviceDoesNotFuzzyMatchOtherLengths(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	_, err := client.LookupDevice(context.Background(), "8696710772024200")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LookupDevice() error = %v, want ErrNotFound", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want exact lookup only", requests)
	}
}

func TestSendSMSAndFetchHistory(t *testing.T) {
	var idempotencyKey string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2.0/sims/send_sms/":
			idempotencyKey = r.Header.Get("Idempotency-Key")
			var request smsSendRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if len(request.ICCIDs) != 1 || request.Text != "getinfo" {
				t.Errorf("send request = %+v", request)
			}
			_ = json.NewEncoder(w).Encode(smsSendResponse{
				ResourceURL: srv.URL + "/api/v2.0/sims/send_sms/status/sms-1/",
				Detail:      "OK",
			})
		case "/api/v2.0/sims/send_sms/status/":
			if r.URL.Query().Get("iccid") != "8944470000000000001" {
				t.Errorf("history query = %s", r.URL.RawQuery)
			}
			var status smsStatusDTO
			status.ID = "sms-1"
			status.Content = "getinfo"
			status.DateSubmitted = "2026-08-05T12:00:00Z"
			status.DeliveryReport = append(status.DeliveryReport, struct {
				ICCID          string `json:"iccid"`
				DeliveryStatus string `json:"deliveryStatus"`
			}{ICCID: "8944470000000000001", DeliveryStatus: "DELIVERED"})
			_ = json.NewEncoder(w).Encode([]smsStatusDTO{status})
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	receipt, err := client.SendSMS(context.Background(), "8944470000000000001", "getinfo")
	if err != nil {
		t.Fatalf("SendSMS() error = %v", err)
	}
	if receipt.ID != "sms-1" || idempotencyKey == "" {
		t.Errorf("receipt = %+v, idempotency key = %q", receipt, idempotencyKey)
	}
	history, err := client.FetchSMSHistory(context.Background(), "8944470000000000001", 20)
	if err != nil {
		t.Fatalf("FetchSMSHistory() error = %v", err)
	}
	if len(history) != 1 || history[0].DeliveryStatus != "DELIVERED" {
		t.Errorf("history = %+v", history)
	}
}

func TestFetchSMSHistoryClassifiesDirection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received := smsStatusDTO{ID: "sms-mo", Content: "\x05\x00\x03\x00\x01\x01SET OK"}
		received.DeliveryReport = append(received.DeliveryReport, struct {
			ICCID          string `json:"iccid"`
			DeliveryStatus string `json:"deliveryStatus"`
		}{ICCID: "8944470000000000001", DeliveryStatus: "DELIVERED"})
		sent := smsStatusDTO{ID: "sms-mt", Content: "TIMER,30,3600#\r\n"}
		sent.DeliveryReport = append(sent.DeliveryReport, struct {
			ICCID          string `json:"iccid"`
			DeliveryStatus string `json:"deliveryStatus"`
		}{ICCID: "8944470000000000001", DeliveryStatus: "DELIVERED"})
		_ = json.NewEncoder(w).Encode([]smsStatusDTO{received, sent})
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/api/")
	client := newClient(baseURL, "provider-secret", srv.Client())
	history, err := client.FetchSMSHistory(context.Background(), "8944470000000000001", 20)
	if err != nil {
		t.Fatalf("FetchSMSHistory() error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].Direction != "MO" || history[0].Content != "SET OK" {
		t.Errorf("received message = %+v", history[0])
	}
	if history[1].Direction != "MT" || history[1].Content != "TIMER,30,3600#" {
		t.Errorf("sent message = %+v", history[1])
	}
}

func TestFetchDeviceUsageRequiresToken(t *testing.T) {
	client := NewClient("")
	_, err := client.LookupDevice(context.Background(), "123")
	if err == nil {
		t.Fatal("LookupDevice() error = nil, want configuration error")
	}
}
