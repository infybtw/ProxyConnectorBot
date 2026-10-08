// Package origin fetches subscriptions from provider (origin) servers,
// attaching the stored HWID so the provider treats the request as coming
// from the bound device.
package origin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// Device describes the fake device identity sent to the origin server.
type Device struct {
	OS        string
	OSVersion string
	Model     string
	UserAgent string
}

// Client fetches subscriptions from origin servers.
type Client struct {
	http   *http.Client
	device Device
	maxLen int64
}

// Result is a raw origin response.
type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// NewClient builds a Client with the given timeout and device identity.
func NewClient(timeout time.Duration, maxLen int64, device Device) *Client {
	return &Client{
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("origin: too many redirects")
				}
				return nil
			},
		},
		device: device,
		maxLen: maxLen,
	}
}

// Fetch requests the subscription from the origin URL, attaching the origin's
// HWID according to its delivery mode (header or query parameter) plus stable
// device headers expected by Happ/INCY style panels.
func (c *Client) Fetch(ctx context.Context, o store.Origin) (*Result, error) {
	target, err := url.Parse(o.URL)
	if err != nil {
		return nil, fmt.Errorf("origin: parse url: %w", err)
	}

	switch o.HWIDMode {
	case store.HWIDModeQuery:
		q := target.Query()
		q.Set(o.HWIDParam, o.HWID)
		target.RawQuery = q.Encode()
	default:
		target.RawQuery = target.Query().Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("origin: build request: %w", err)
	}

	if o.HWIDMode != store.HWIDModeQuery {
		req.Header.Set(o.HWIDParam, o.HWID)
	}
	req.Header.Set("x-device-os", c.device.OS)
	req.Header.Set("x-ver-os", c.device.OSVersion)
	req.Header.Set("x-device-model", c.device.Model)
	req.Header.Set("Accept", "*/*")
	if c.device.UserAgent != "" {
		req.Header.Set("User-Agent", c.device.UserAgent)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("origin: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxLen+1))
	if err != nil {
		return nil, fmt.Errorf("origin: read body: %w", err)
	}
	if int64(len(body)) > c.maxLen {
		return nil, fmt.Errorf("origin: response exceeds %d bytes", c.maxLen)
	}

	return &Result{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       body,
	}, nil
}

// IsHTTPURL reports whether s looks like an absolute http(s) URL.
func IsHTTPURL(s string) bool {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}
