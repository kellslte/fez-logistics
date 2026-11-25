package fez

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
)

func newAutoMock(t *testing.T, pw string) (*mockFez, *Client) {
	m := &mockFez{t: t, orders: map[string]map[string]any{}, status: map[string]string{}, wallet: true, tok: "TOK"}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return m, New(WithBaseURL(srv.URL), WithAutoLogin("me@x.com", pw))
}

func (m *mockFez) expire(newTok string) { m.mu.Lock(); m.tok = newTok; m.mu.Unlock() }
func (m *mockFez) loginCount() int      { m.mu.Lock(); defer m.mu.Unlock(); return m.logins }

func TestAutoLoginLazy(t *testing.T) {
	m, c := newAutoMock(t, "pw")
	if _, err := c.Users(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.loginCount() != 1 {
		t.Fatalf("logins = %d, want 1", m.loginCount())
	}
	// /states is public: must not trigger a login on a fresh client.
	m2, c2 := newAutoMock(t, "pw")
	if _, err := c2.States(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m2.loginCount() != 0 {
		t.Fatal("/states triggered a login")
	}
}

func TestAutoLoginRetriesAfterExpiry(t *testing.T) {
	m, c := newAutoMock(t, "pw")
	ctx := context.Background()
	c.Users(ctx)
	m.expire("TOK2") // server-side expiry: old token now 401s
	if _, err := c.Users(ctx); err != nil {
		t.Fatalf("call after expiry: %v", err)
	}
	if m.loginCount() != 2 {
		t.Fatalf("logins = %d, want 2", m.loginCount())
	}
}

func TestAutoLoginConcurrent401sShareOneLogin(t *testing.T) {
	m, c := newAutoMock(t, "pw")
	ctx := context.Background()
	c.Users(ctx)
	m.expire("TOK2")
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := c.Users(ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := m.loginCount(); n != 2 {
		t.Fatalf("logins = %d, want 2 (one initial, one shared re-login)", n)
	}
}

func TestAutoLoginBadPasswordSurfaces(t *testing.T) {
	_, c := newAutoMock(t, "wrong")
	_, err := c.Users(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.HTTPStatus != 401 {
		t.Fatalf("got %v", err)
	}
}

func TestLogoutClearsTokenAndAutoLoginResumes(t *testing.T) {
	m, c := newAutoMock(t, "pw")
	ctx := context.Background()
	c.Users(ctx)
	if err := c.Logout(ctx, ""); err != nil {
		t.Fatal(err)
	}
	c.mu.RLock()
	tok := c.token
	c.mu.RUnlock()
	if tok != "" {
		t.Fatal("token kept after logout")
	}
	if _, err := c.Users(ctx); err != nil {
		t.Fatal(err)
	}
	if m.loginCount() != 2 {
		t.Fatalf("logins = %d, want 2", m.loginCount())
	}
}

func TestAuthenticateEnablesAutoLoginByDefault(t *testing.T) {
	m, c := newMock(t)
	ctx := context.Background()
	if _, err := c.Authenticate(ctx, "me@x.com", "pw"); err != nil {
		t.Fatal(err)
	}
	m.expire("TOK2")
	if _, err := c.Users(ctx); err != nil {
		t.Fatalf("expired token not renewed: %v", err)
	}
	if m.loginCount() != 2 {
		t.Fatalf("logins = %d, want 2", m.loginCount())
	}
}

func TestFailedAuthenticateRemembersNothing(t *testing.T) {
	m, c := newMock(t)
	c.Authenticate(context.Background(), "me@x.com", "wrong")
	if _, err := c.Users(context.Background()); err == nil || m.loginCount() != 0 {
		t.Fatalf("err=%v logins=%d", err, m.loginCount())
	}
}

func TestWithoutAutoLogin(t *testing.T) {
	m := &mockFez{t: t, orders: map[string]map[string]any{}, status: map[string]string{}, wallet: true, tok: "TOK"}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	c := New(WithBaseURL(srv.URL), WithoutAutoLogin())
	ctx := context.Background()
	c.Authenticate(ctx, "me@x.com", "pw")
	if c.loginPW != "" {
		t.Fatal("password kept in memory")
	}
	m.expire("TOK2")
	_, err := c.Users(ctx)
	var ae *APIError
	if !errors.As(err, &ae) || ae.HTTPStatus != 401 {
		t.Fatalf("got %v", err)
	}
}

func TestLogoutAfterAuthenticateStopsAutoLogin(t *testing.T) {
	m, c := newMock(t)
	ctx := context.Background()
	c.Authenticate(ctx, "me@x.com", "pw")
	if err := c.Logout(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Users(ctx); err == nil {
		t.Fatal("call after Logout succeeded; client logged back in")
	}
	if m.loginCount() != 1 {
		t.Fatalf("logins = %d, want 1", m.loginCount())
	}
}

func TestChangePasswordUpdatesStoredPassword(t *testing.T) {
	_, c := newMock(t)
	ctx := context.Background()
	c.Authenticate(ctx, "me@x.com", "pw")
	if err := c.ChangePassword(ctx, "G-other", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if c.loginPW != "pw" {
		t.Fatal("changing another user's password overwrote ours")
	}
	if err := c.ChangePassword(ctx, "", "pw", "new"); err != nil {
		t.Fatal(err)
	}
	if c.loginPW != "new" {
		t.Fatalf("stored password = %q, want new", c.loginPW)
	}
}
