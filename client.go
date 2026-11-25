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
	"errors"
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

	// Auto-login credentials, guarded by mu. Set by WithAutoLogin or by a
	// successful Authenticate. loginMu serialises re-logins so concurrent 401s
	// trigger one /user/authenticate call, not one each.
	loginID, loginPW string
	loginPinned      bool // set via WithAutoLogin: survives Logout
	noAutoLogin      bool // WithoutAutoLogin
	loginMu          sync.Mutex
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

// Session handling is automatic. Fez has no refresh-token endpoint; logging in
// again is the only way to renew a session. So after a successful
// Authenticate the client keeps the credentials in memory, and when a call gets
// HTTP 401 (expired token) it logs in again once and retries that call.
// Concurrent 401s share one login, keeping well under Fez's 60 logins/minute.
// Retrying is safe for order creation: a 401 is returned before Fez processes
// the request.

// WithAutoLogin supplies the credentials up front, so you never call
// Authenticate: the client logs in before its first authenticated call. Unlike
// credentials remembered from Authenticate, these survive Logout.
func WithAutoLogin(userID, password string) Option {
	return func(c *Client) { c.loginID, c.loginPW, c.loginPinned = userID, password, true }
}

// WithoutAutoLogin disables automatic re-login: the password is never kept in
// memory and an expired token surfaces as a 401 *APIError.
func WithoutAutoLogin() Option { return func(c *Client) { c.noAutoLogin = true } }

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
// describe with a request body). When login credentials are known it also logs
// in on demand and retries once after a 401.
func (c *Client) do(ctx context.Context, method, path string, body, out any, query bool) error {
	if c.cfgErr != nil {
		return c.cfgErr
	}
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
	}
	c.mu.RLock()
	auto := c.loginID != "" && !c.noAutoLogin && path != "/user/authenticate" && path != "/states"
	empty := c.token == ""
	c.mu.RUnlock()
	if auto {
		if empty {
			if err := c.relogin(ctx, ""); err != nil {
				return err
			}
		}
	}
	used, err := c.send(ctx, method, path, raw, body != nil, out, query)
	var ae *APIError
	if auto && errors.As(err, &ae) && ae.HTTPStatus == http.StatusUnauthorized {
		if lerr := c.relogin(ctx, used); lerr != nil {
			return fmt.Errorf("fez: re-login after 401 failed: %w (original: %v)", lerr, err)
		}
		_, err = c.send(ctx, method, path, raw, body != nil, out, query)
	}
	return err
}

// relogin authenticates with the stored credentials unless another
// goroutine already replaced staleToken while we waited for the lock.
func (c *Client) relogin(ctx context.Context, staleToken string) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	c.mu.RLock()
	cur, id, pw := c.token, c.loginID, c.loginPW
	c.mu.RUnlock()
	if cur != "" && cur != staleToken {
		return nil
	}
	_, err := c.Authenticate(ctx, id, pw)
	return err
}

// send performs one HTTP round trip and returns the bearer token it used.
func (c *Client) send(ctx context.Context, method, path string, raw []byte, hasBody bool, out any, query bool) (string, error) {
	var rdr io.Reader
	if hasBody {
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
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "fez-logistics-go/"+Version)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	c.mu.RLock()
	token := c.token
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if c.secretKey != "" {
		req.Header.Set("secret-key", c.secretKey)
	}
	c.mu.RUnlock()

	resp, err := c.http.Do(req)
	if err != nil {
		return token, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return token, err
	}
	var env Envelope
	_ = json.Unmarshal(data, &env)
	if resp.StatusCode >= 300 || env.Status == "Error" {
		return token, &APIError{HTTPStatus: resp.StatusCode, Description: env.Description, Body: string(data)}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return token, fmt.Errorf("fez: decode response: %w", err)
		}
	}
	return token, nil
}
