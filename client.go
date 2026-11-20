// Package fez is a Go client for the Fez Delivery Business (corporate) API.
//
// Usage:
//
//	c := fez.New()
//	if _, err := c.Authenticate(ctx, userID, password); err != nil { ... }
//	res, err := c.CreateOrders(ctx, []fez.CreateOrder{{...}})
package fez

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// SandboxURL is the documented development base URL. The production base URL
// is not published in the docs; pass it with WithBaseURL.
const SandboxURL = "https://apisandbox.fezdelivery.co/v1"

// Client talks to the Fez API. Safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client

	mu        sync.RWMutex
	token     string
	secretKey string
	expires   string
	userID    string
}

// Option configures a Client.
type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = u } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithCredentials(token, secretKey string) Option {
	return func(c *Client) { c.token, c.secretKey = token, secretKey }
}

// New returns a client pointed at the sandbox unless WithBaseURL is given.
func New(opts ...Option) *Client {
	c := &Client{baseURL: SandboxURL, http: &http.Client{Timeout: 30 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// SetCredentials sets the bearer token and organisation secret key manually.
func (c *Client) SetCredentials(token, secretKey string) {
	c.mu.Lock()
	c.token, c.secretKey = token, secretKey
	c.mu.Unlock()
}

// TokenExpiry is the expiry returned by the last Authenticate call.
func (c *Client) TokenExpiry() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.expires
}

// APIError is returned for non-2xx responses or {"status":"Error"} bodies.
type APIError struct {
	HTTPStatus  int
	Description string
	Body        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fez: http %d: %s", e.HTTPStatus, e.Description)
}

// Envelope is the common response header.
type Envelope struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

// do sends a request. body (if non-nil) is JSON-encoded. When query is true the
// body fields are ALSO sent as URL query parameters (for GET endpoints the docs
// describe with a request body).
func (c *Client) do(ctx context.Context, method, path string, body, out any, query bool) error {
	var rdr io.Reader
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	u := c.baseURL + path
	if query && raw != nil {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			q := url.Values{}
			for k, v := range m {
				q.Set(k, fmt.Sprint(v))
			}
			u += "?" + q.Encode()
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "fez-logistics-go/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.mu.RLock()
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.secretKey != "" {
		req.Header.Set("secret-key", c.secretKey)
	}
	c.mu.RUnlock()

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env Envelope
	_ = json.Unmarshal(data, &env)
	if resp.StatusCode >= 300 || env.Status == "Error" {
		return &APIError{HTTPStatus: resp.StatusCode, Description: env.Description, Body: string(data)}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("fez: decode response: %w", err)
		}
	}
	return nil
}
