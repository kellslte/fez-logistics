package fez

import (
	"context"
	"os"
	"testing"
	"time"
)

// Tests Logout and RegisterWebhook. Mutates sandbox account state:
//   - Logout invalidates the session token (then we verify it is rejected).
//   - RegisterWebhook (only if FEZ_TEST_WEBHOOK_URL is set) REPLACES the org's
//     webhook; there is no read/delete API.
//
// Runs only with FEZ_WRITE_TESTS=1.
func TestSandboxSession(t *testing.T) {
	loadEnv()
	uid, pw := os.Getenv("FEZ_USER_ID"), os.Getenv("FEZ_PASSWORD")
	if os.Getenv("FEZ_WRITE_TESTS") != "1" || uid == "" {
		t.Skip("set FEZ_WRITE_TESTS=1 and credentials to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := New()
	if _, err := c.Authenticate(ctx, uid, pw); err != nil {
		t.Fatal(err)
	}

	// RegisterWebhook overwrites the account's webhook, so only run it when a
	// URL is given explicitly (never clobber a URL you set on the dashboard).
	if hook := os.Getenv("FEZ_TEST_WEBHOOK_URL"); hook == "" {
		t.Log("skipping RegisterWebhook: set FEZ_TEST_WEBHOOK_URL to test it")
	} else {
		r, err := c.RegisterWebhook(ctx, hook)
		if err != nil {
			t.Fatalf("RegisterWebhook: %v", err)
		}
		if r.Data.Webhook != hook {
			t.Errorf("webhook = %q want %q", r.Data.Webhook, hook)
		}
		t.Logf("RegisterWebhook ok: %+v", r.Data)
	}

	if err := c.Logout(ctx, ""); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	t.Log("Logout ok")
	if _, err := c.Users(ctx); err == nil {
		t.Error("token still accepted after Logout")
	} else {
		t.Logf("post-logout call rejected as expected: %v", err)
	}
	// (/states is public, so Users is used to check the token.) Re-auth must work.
	if _, err := c.Authenticate(ctx, uid, pw); err != nil {
		t.Fatalf("re-authenticate: %v", err)
	}
	if _, err := c.Users(ctx); err != nil {
		t.Errorf("Users after re-auth: %v", err)
	}
}
