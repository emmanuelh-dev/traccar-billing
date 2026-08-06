package truphone

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

var (
	ErrNotFound     = errors.New("1global: device not found")
	ErrUnauthorized = errors.New("1global: unauthorized")
)

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func (*Client) ID() string { return "1global" }

func NewClient(token string) *Client {
	baseURL, _ := url.Parse("https://iot.truphone.com/api/")
	return &Client{
		baseURL:    baseURL,
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// newClient is kept private because overriding the provider URL is useful for
// tests, but production should always use the official 1GLOBAL endpoint.
func newClient(baseURL *url.URL, token string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, token: token, httpClient: httpClient}
}

type deviceDTO struct {
	IMEI  string `json:"imei"`
	ICCID string `json:"iccid"`
}

type subscriptionDTO struct {
	Subscription struct {
		ServicePackID      string `json:"servicePackId"`
		SubscriptionStatus string `json:"subscriptionStatus"`
	} `json:"subscription"`
	Dates struct {
		FirstActivationDate string `json:"firstActivationDate"`
	} `json:"dates"`
}

type simDTO struct {
	IMEI         string `json:"imei"`
	ICCID        string `json:"iccid"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Subscription struct {
		ServicePackID      string `json:"servicePackId"`
		SubscriptionStatus string `json:"subscriptionStatus"`
	} `json:"subscription"`
	Dates struct {
		FirstActivationDate string `json:"firstActivationDate"`
	} `json:"dates"`
	// FirstActivationDate is the v2.2 shape: the same value moved to the top
	// level instead of being nested under Dates.
	FirstActivationDate string `json:"firstActivationDate"`
}

func (s simDTO) activatedAt() string {
	if s.FirstActivationDate != "" {
		return s.FirstActivationDate
	}
	return s.Dates.FirstActivationDate
}

const simListPageSize = 500
const simListMaxPages = 20

func (c *Client) ListSIMs(ctx context.Context) ([]billing.SIMInfo, error) {
	if c.token == "" {
		return nil, fmt.Errorf("1global: token is not configured")
	}
	// v2.2 is the version documented for the SIM list, but tokens are scoped
	// per version: accounts provisioned for v2.0 get 401 there, not 404. Keep
	// v2.0 as the working default and only reach for v2.2 if it disappears.
	sims, err := c.listAllSIMs(ctx, "v2.0")
	if errors.Is(err, ErrNotFound) {
		sims, err = c.listAllSIMs(ctx, "v2.2")
	}
	if err != nil {
		return nil, fmt.Errorf("1global: list sims: %w", err)
	}
	result := make([]billing.SIMInfo, len(sims))
	for i, sim := range sims {
		result[i] = billing.SIMInfo{
			IMEI: sim.IMEI, ICCID: sim.ICCID, Label: sim.Label, Description: sim.Description,
			Status: sim.Subscription.SubscriptionStatus, ServicePlan: sim.Subscription.ServicePackID,
			ActivatedAt: sim.activatedAt(),
		}
	}
	return result, nil
}

func (c *Client) listAllSIMs(ctx context.Context, version string) ([]simDTO, error) {
	var all []simDTO
	for page := 1; page <= simListMaxPages; page++ {
		sims, err := c.listSIMsPage(ctx, version, page)
		if err != nil {
			return nil, err
		}
		all = append(all, sims...)
		if len(sims) < simListPageSize {
			break
		}
	}
	return all, nil
}

func (c *Client) listSIMsPage(ctx context.Context, version string, page int) ([]simDTO, error) {
	endpoint := c.endpoint(version, "sims")
	endpoint.RawQuery = url.Values{
		"per_page": {strconv.Itoa(simListPageSize)},
		"page":     {strconv.Itoa(page)},
	}.Encode()
	var sims []simDTO
	if err := c.getJSON(ctx, endpoint, &sims); err != nil {
		return nil, err
	}
	return sims, nil
}

func (c *Client) LookupDevice(ctx context.Context, imei string) (billing.SIMInfo, error) {
	imei = strings.TrimSpace(imei)
	if imei == "" {
		return billing.SIMInfo{}, fmt.Errorf("1global: empty imei")
	}
	if c.token == "" {
		return billing.SIMInfo{}, fmt.Errorf("1global: token is not configured")
	}

	device, err := c.fetchDevice(ctx, imei)
	if ((err == nil && device.ICCID == "") || errors.Is(err, ErrNotFound)) && isFifteenDigitIMEI(imei) {
		// 1GLOBAL commonly stores the 14-digit TAC+serial while Traccar stores
		// the complete 15-digit IMEI with its Luhn check digit.
		device, err = c.fetchDevice(ctx, imei[:14])
	}
	if err != nil {
		return billing.SIMInfo{}, fmt.Errorf("1global: fetch device %s: %w", imei, err)
	}
	if device.ICCID == "" {
		return billing.SIMInfo{}, fmt.Errorf("1global: device %s has no iccid", imei)
	}

	var subscription subscriptionDTO
	if err := c.getJSON(ctx, c.endpoint("v2.0", "sims", device.ICCID), &subscription); err != nil {
		return billing.SIMInfo{}, fmt.Errorf("1global: fetch sim %s: %w", device.ICCID, err)
	}

	return billing.SIMInfo{
		IMEI:        device.IMEI,
		ICCID:       device.ICCID,
		Status:      subscription.Subscription.SubscriptionStatus,
		ServicePlan: subscription.Subscription.ServicePackID,
		ActivatedAt: subscription.Dates.FirstActivationDate,
	}, nil
}

func (c *Client) fetchDevice(ctx context.Context, imei string) (deviceDTO, error) {
	var device deviceDTO
	err := c.getJSON(ctx, c.endpoint("v2.2", "devices", imei), &device)
	return device, err
}

func isFifteenDigitIMEI(imei string) bool {
	if len(imei) != 15 {
		return false
	}
	for _, char := range imei {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

type smsSendRequest struct {
	ICCIDs []string `json:"iccid"`
	Text   string   `json:"text"`
}

type smsSendResponse struct {
	ResourceURL string `json:"resourceURL"`
	Detail      string `json:"detail"`
}

type smsStatusDTO struct {
	ID             string `json:"id"`
	DateSubmitted  string `json:"dateSubmitted"`
	Content        string `json:"content"`
	DeliveryReport []struct {
		ICCID          string `json:"iccid"`
		DeliveryStatus string `json:"deliveryStatus"`
	} `json:"deliveryReport"`
}

func (c *Client) SendSMS(ctx context.Context, iccid, text string) (billing.SMSReceipt, error) {
	iccid = strings.TrimSpace(iccid)
	text = strings.TrimSpace(text)
	if iccid == "" || text == "" {
		return billing.SMSReceipt{}, fmt.Errorf("1global: iccid and sms text are required")
	}

	payload, err := json.Marshal(smsSendRequest{ICCIDs: []string{iccid}, Text: text})
	if err != nil {
		return billing.SMSReceipt{}, fmt.Errorf("1global: encode sms: %w", err)
	}
	var response smsSendResponse
	if err := c.doJSONWithIdempotency(ctx, http.MethodPost, c.endpoint("v2.0", "sims", "send_sms"), bytes.NewReader(payload), &response); err != nil {
		return billing.SMSReceipt{}, fmt.Errorf("1global: send sms: %w", err)
	}
	id := strings.Trim(strings.TrimSpace(response.ResourceURL), "/")
	if slash := strings.LastIndex(id, "/"); slash >= 0 {
		id = id[slash+1:]
	}
	return billing.SMSReceipt{ID: id, Detail: response.Detail}, nil
}

func (c *Client) FetchSMSHistory(ctx context.Context, iccid string, limit int) ([]billing.SMSMessage, error) {
	iccid = strings.TrimSpace(iccid)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	endpoint := c.endpoint("v2.0", "sims", "send_sms", "status")
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "o": {"-id"}}
	if iccid != "" {
		query.Set("iccid", iccid)
	}
	endpoint.RawQuery = query.Encode()

	var statuses []smsStatusDTO
	if err := c.getJSON(ctx, endpoint, &statuses); err != nil {
		return nil, fmt.Errorf("1global: fetch sms history: %w", err)
	}
	messages := make([]billing.SMSMessage, 0, len(statuses))
	for _, status := range statuses {
		for _, report := range status.DeliveryReport {
			if iccid == "" || report.ICCID == iccid {
				messages = append(messages, billing.SMSMessage{
					ID: status.ID, DateSubmitted: status.DateSubmitted, Content: status.Content,
					ICCID: report.ICCID, DeliveryStatus: report.DeliveryStatus,
				})
			}
		}
	}
	return messages, nil
}

type changeStatusRequest struct {
	ICCIDs []string `json:"iccid"`
	Status string   `json:"status"`
}

type changeStatusResponse struct {
	Detail string `json:"detail"`
}

func (c *Client) ChangeSIMStatus(ctx context.Context, iccids []string, status string) error {
	trimmed := make([]string, 0, len(iccids))
	for _, iccid := range iccids {
		if iccid = strings.TrimSpace(iccid); iccid != "" {
			trimmed = append(trimmed, iccid)
		}
	}
	if len(trimmed) == 0 || status == "" {
		return fmt.Errorf("1global: iccid and status are required")
	}

	payload, err := json.Marshal(changeStatusRequest{ICCIDs: trimmed, Status: status})
	if err != nil {
		return fmt.Errorf("1global: change sim status: %w", err)
	}
	var response changeStatusResponse
	if err := c.doJSONWithIdempotency(ctx, http.MethodPost, c.endpoint("v2.0", "sims", "change_status"), bytes.NewReader(payload), &response); err != nil {
		return fmt.Errorf("1global: change sim status: %w", err)
	}
	return nil
}

type reportRequest struct {
	ReportType   string   `json:"report_type"`
	ICCIDs       []string `json:"iccid"`
	Granularity  string   `json:"granularity"`
	Period       string   `json:"period,omitempty"`
	StartDate    string   `json:"startDate,omitempty"`
	EndDate      string   `json:"endDate,omitempty"`
	OutputFormat string   `json:"output_format"`
}

type reportResponse struct {
	DownloadURL string `json:"downloadUrl"`
}

type reportRow struct {
	ICCID      string `json:"ICCID"`
	TotalUsage string `json:"Total Usage"`
	StartDate  string `json:"Start Date"`
	EndDate    string `json:"End Date"`
}

func (c *Client) FetchUsage(ctx context.Context, iccids []string) ([]billing.UsageRecord, error) {
	return c.fetchUsageReport(ctx, reportRequest{
		ReportType: "DATA_USAGE", ICCIDs: iccids, Granularity: "Day",
		Period: "PAST_30_DAYS", OutputFormat: "JSON",
	})
}

func (c *Client) FetchUsageRange(ctx context.Context, iccids []string, start, end time.Time) ([]billing.UsageRecord, error) {
	return c.fetchUsageReport(ctx, reportRequest{
		ReportType: "DATA_USAGE", ICCIDs: iccids, Granularity: "Month",
		StartDate: start.UTC().Format("2006-01-02 15:04:05"),
		EndDate:   end.UTC().Format("2006-01-02 15:04:05"), OutputFormat: "JSON",
	})
}

func (c *Client) fetchUsageReport(ctx context.Context, request reportRequest) ([]billing.UsageRecord, error) {
	iccids := request.ICCIDs
	if len(iccids) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("1global: encode usage report: %w", err)
	}

	var generated reportResponse
	if err := c.doJSON(ctx, http.MethodPost, c.endpoint("v2.0", "reports", "generate"), bytes.NewReader(payload), &generated); err != nil {
		return nil, fmt.Errorf("1global: generate usage report: %w", err)
	}
	downloadURL, err := url.Parse(generated.DownloadURL)
	if err != nil || downloadURL.Scheme != c.baseURL.Scheme || downloadURL.Host != c.baseURL.Host {
		return nil, fmt.Errorf("1global: invalid report download url")
	}

	var rows []reportRow
	if err := c.getJSON(ctx, downloadURL, &rows); err != nil {
		return nil, fmt.Errorf("1global: download usage report: %w", err)
	}
	result := make([]billing.UsageRecord, len(rows))
	for i, row := range rows {
		result[i] = billing.UsageRecord{
			ICCID:      row.ICCID,
			TotalUsage: normalizeReportUsage(row.TotalUsage),
			StartDate:  row.StartDate,
			EndDate:    row.EndDate,
		}
	}
	return result, nil
}

// DATA_USAGE report totals are returned as bare kilobyte values, unlike the
// allowance examples elsewhere in the API which include explicit units.
func normalizeReportUsage(raw string) string {
	raw = strings.TrimSpace(raw)
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw + " KB"
	}
	return raw
}

func (c *Client) endpoint(parts ...string) *url.URL {
	endpoint := c.baseURL.JoinPath(parts...)
	// The IoT Portal API only accepts resource URIs ending in a slash.
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/"
	return endpoint
}

func (c *Client) getJSON(ctx context.Context, endpoint *url.URL, out any) error {
	return c.doJSON(ctx, http.MethodGet, endpoint, nil, out)
}

func (c *Client) doJSON(ctx context.Context, method string, endpoint *url.URL, body io.Reader, out any) error {
	return c.doJSONRequest(ctx, method, endpoint, body, out, "")
}

func (c *Client) doJSONWithIdempotency(ctx context.Context, method string, endpoint *url.URL, body io.Reader, out any) error {
	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		return fmt.Errorf("create idempotency key: %w", err)
	}
	return c.doJSONRequest(ctx, method, endpoint, body, out, hex.EncodeToString(keyBytes[:]))
}

func (c *Client) doJSONRequest(ctx context.Context, method string, endpoint *url.URL, body io.Reader, out any, idempotencyKey string) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
