package traccar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// defaultSessionTTL is used when Traccar's session cookie carries no
// explicit expiry (its default JSESSIONID is a plain session cookie).
// ponytail: fixed TTL ceiling; switch to reading the cookie's Max-Age if a
// future Traccar version starts setting one.
const defaultSessionTTL = 12 * time.Hour

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Login(ctx context.Context, baseURL *url.URL, email, password string) (billing.Session, billing.TraccarUser, error) {
	endpoint := baseURL.JoinPath("session")

	form := url.Values{"email": {email}, "password": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return billing.Session{}, billing.TraccarUser{}, fmt.Errorf("traccar: build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return billing.Session{}, billing.TraccarUser{}, fmt.Errorf("traccar: login request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return billing.Session{}, billing.TraccarUser{}, err
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return billing.Session{}, billing.TraccarUser{}, fmt.Errorf("traccar: login succeeded but no session cookie was returned")
	}
	sessionCookie := cookies[0]

	expiresAt := sessionCookie.Expires
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(defaultSessionTTL)
	}

	var dto userDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return billing.Session{}, billing.TraccarUser{}, fmt.Errorf("traccar: decode login response: %w", err)
	}

	return billing.Session{
		Cookie:    fmt.Sprintf("%s=%s", sessionCookie.Name, sessionCookie.Value),
		ExpiresAt: expiresAt,
	}, billing.TraccarUser{ID: dto.ID, Name: dto.Name, Email: dto.Email}, nil
}

func (c *Client) FetchUsers(ctx context.Context, baseURL *url.URL, session billing.Session) ([]billing.TraccarUser, error) {
	var dtos []userDTO
	if err := c.getJSON(ctx, baseURL.JoinPath("users"), session, &dtos); err != nil {
		return nil, fmt.Errorf("traccar: fetch users: %w", err)
	}

	users := make([]billing.TraccarUser, len(dtos))
	for i, d := range dtos {
		users[i] = billing.TraccarUser{ID: d.ID, Name: d.Name, Email: d.Email, Disabled: d.Disabled}
	}
	return users, nil
}

func (c *Client) FetchDevices(ctx context.Context, baseURL *url.URL, session billing.Session) ([]billing.TraccarDevice, error) {
	endpoint := baseURL.JoinPath("devices")
	endpoint.RawQuery = url.Values{"all": {"true"}}.Encode()
	return c.fetchDevices(ctx, endpoint, session)
}

func (c *Client) FetchDeviceProtocols(ctx context.Context, baseURL *url.URL, session billing.Session) (map[int64]string, error) {
	var positions []positionDTO
	if err := c.getJSON(ctx, baseURL.JoinPath("positions"), session, &positions); err != nil {
		return nil, fmt.Errorf("traccar: fetch latest positions: %w", err)
	}

	protocols := make(map[int64]string, len(positions))
	for _, position := range positions {
		protocols[position.DeviceID] = position.Protocol
	}
	return protocols, nil
}

func (c *Client) FetchDevicesForUser(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64) ([]billing.TraccarDevice, error) {
	endpoint := baseURL.JoinPath("devices")
	endpoint.RawQuery = url.Values{"userId": {strconv.FormatInt(traccarUserID, 10)}}.Encode()
	return c.fetchDevices(ctx, endpoint, session)
}

func (c *Client) fetchDevices(ctx context.Context, endpoint *url.URL, session billing.Session) ([]billing.TraccarDevice, error) {
	var dtos []deviceDTO
	if err := c.getJSON(ctx, endpoint, session, &dtos); err != nil {
		return nil, fmt.Errorf("traccar: fetch devices: %w", err)
	}

	devices := make([]billing.TraccarDevice, len(dtos))
	for i, d := range dtos {
		devices[i] = billing.TraccarDevice{ID: d.ID, Name: d.Name, UniqueID: d.UniqueID, Status: d.Status}
	}
	return devices, nil
}

func (c *Client) FetchServerInfo(ctx context.Context, baseURL *url.URL, session billing.Session) (billing.TraccarServerInfo, error) {
	var dto serverDTO
	if err := c.getJSON(ctx, baseURL.JoinPath("server"), session, &dto); err != nil {
		return billing.TraccarServerInfo{}, fmt.Errorf("traccar: fetch server info: %w", err)
	}
	return billing.TraccarServerInfo{Version: dto.Version}, nil
}

// SetUserDisabled flips only the "disabled" field on the user's raw JSON
// representation and PUTs the whole object back, since Traccar's PUT
// /users/{id} replaces the entire resource: sending a narrower payload
// would silently wipe every field this client doesn't model.
func (c *Client) SetUserDisabled(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64, disabled bool) error {
	var rawUsers []map[string]any
	if err := c.getJSON(ctx, baseURL.JoinPath("users"), session, &rawUsers); err != nil {
		return fmt.Errorf("traccar: fetch user %d for disable: %w", traccarUserID, err)
	}

	var target map[string]any
	for _, u := range rawUsers {
		if id, ok := u["id"].(float64); ok && int64(id) == traccarUserID {
			target = u
			break
		}
	}
	if target == nil {
		return fmt.Errorf("traccar: user %d not found", traccarUserID)
	}
	target["disabled"] = disabled

	body, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("traccar: encode user %d: %w", traccarUserID, err)
	}

	endpoint := baseURL.JoinPath("users", strconv.FormatInt(traccarUserID, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("traccar: build disable request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	authorize(req, session)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("traccar: disable user %d: %w", traccarUserID, err)
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// DeleteUser removes the user from Traccar. Traccar cascades the deletion
// to that user's permissions, so devices it owned are left server-side
// without an owner rather than deleted.
func (c *Client) DeleteUser(ctx context.Context, baseURL *url.URL, session billing.Session, traccarUserID int64) error {
	endpoint := baseURL.JoinPath("users", strconv.FormatInt(traccarUserID, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("traccar: build delete request: %w", err)
	}
	authorize(req, session)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("traccar: delete user %d: %w", traccarUserID, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return checkStatus(resp)
}

func (c *Client) getJSON(ctx context.Context, endpoint *url.URL, session billing.Session, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	authorize(req, session)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// authorize is the single place that decides how a request proves who it is.
// A token is preferred because it survives the login cookie, and it is sent as
// a Bearer header rather than a ?token= query parameter so it stays out of
// access logs and out of the Referer of anything Traccar redirects to.
func authorize(req *http.Request, session billing.Session) {
	if session.Token != "" {
		req.Header.Set("Authorization", "Bearer "+session.Token)
		return
	}
	req.Header.Set("Cookie", session.Cookie)
}

// CreateToken asks Traccar for an API token for the session's own user.
//
// Traccar caps how far out the expiration may sit and silently shortens
// anything beyond it, so the caller gets back whatever the server agreed to
// rather than what it asked for. The response body is the bare token, though
// some versions quote it as a JSON string.
func (c *Client) CreateToken(ctx context.Context, baseURL *url.URL, session billing.Session, expiresAt time.Time) (string, error) {
	form := url.Values{"expiration": {expiresAt.UTC().Format(time.RFC3339)}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.JoinPath("session", "token").String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("traccar: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authorize(req, session)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("traccar: create token: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return "", fmt.Errorf("traccar: create token: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("traccar: read token: %w", err)
	}

	token := strings.TrimSpace(string(body))
	token = strings.Trim(token, `"`)
	if token == "" {
		return "", fmt.Errorf("traccar: server returned an empty token")
	}
	return token, nil
}

func checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: status %d", ErrUnauthorized, resp.StatusCode)
	default:
		return fmt.Errorf("traccar: unexpected status %d", resp.StatusCode)
	}
}
