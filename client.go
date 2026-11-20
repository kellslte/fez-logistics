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
	"strings"
	"sync"
	"time"
)

// Base URLs for each environment.
const (
	SandboxURL    = "https://apisandbox.fezdelivery.co/v1"
	ProductionURL = "https://api.fezdelivery.co/v1"
)

// Environment selects which Fez API the client targets.
type Environment string

const (
	Sandbox    Environment = "sandbox"
	Production Environment = "production"
)

// BaseURL returns the API base URL for the environment ("" if unknown).
func (e Environment) BaseURL() string {
	switch e {
	case Sandbox:
		return SandboxURL
	case Production:
		return ProductionURL
	}
	return ""
}

// ParseEnvironment converts a string such as an env var ("sandbox",
// "production", case-insensitive; "prod" and "dev" accepted) to an Environment.
func ParseEnvironment(s string) (Environment, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sandbox", "dev", "development", "test":
		return Sandbox, nil
	case "production", "prod", "live":
		return Production, nil
	}
	return "", fmt.Errorf("fez: unknown environment %q (want sandbox or production)", s)
}

// Client talks to the Fez API. Safe for concurrent use.
type Client struct {
	env     Environment
	baseURL string
	http    *http.Client
	cfgErr  error
	urlSet  bool

	mu        sync.RWMutex
	token     string
	secretKey string
	expires   string
	userID    string
}

// Option configures a Client.
type Option func(*Client)

// WithEnvironment targets Sandbox (the default) or Production. An unknown
// value makes every call fail rather than silently using another environment.
func WithEnvironment(e Environment) Option {
	return func(c *Client) {
		if u := e.BaseURL(); u != "" {
			c.env = e
			if !c.urlSet {
				c.baseURL = u
			}
		} else {
			c.cfgErr = fmt.Errorf("fez: unknown environment %q (want sandbox or production)", e)
		}
	}
}

// WithBaseURL overrides the base URL (e.g. a proxy or test server); it takes
// precedence over WithEnvironment regardless of option order.
func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL, c.urlSet = u, true } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithCredentials(token, secretKey string) Option {
	return func(c *Client) { c.token, c.secretKey = token, secretKey }
}

// New returns a client targeting Sandbox unless WithEnvironment/WithBaseURL say otherwise.
func New(opts ...Option) *Client {
	c := &Client{env: Sandbox, baseURL: SandboxURL, http: &http.Client{Timeout: 30 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Environment reports the targeted environment.
func (c *Client) Environment() Environment { return c.env }

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
	if c.cfgErr != nil {
		return c.cfgErr
	}
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
