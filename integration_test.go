package fez

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// loadEnv reads a local .env (KEY=VALUE) without overriding real env vars.
func loadEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && !strings.HasPrefix(k, "#") && os.Getenv(k) == "" {
			os.Setenv(k, strings.Trim(v, `"'`))
		}
	}
}

// Read-only sandbox smoke test; skipped unless FEZ_USER_ID/FEZ_PASSWORD are set.
func TestSandboxReadOnly(t *testing.T) {
	loadEnv()
	uid, pw := os.Getenv("FEZ_USER_ID"), os.Getenv("FEZ_PASSWORD")
	if uid == "" || pw == "" {
		t.Skip("FEZ_USER_ID/FEZ_PASSWORD not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := New()
	auth, err := c.Authenticate(ctx, uid, pw)
	if err != nil {
		t.Fatal("authenticate:", err)
	}
	t.Logf("dashboard key == returned secret key: %v", os.Getenv("FEZ_SECRET_KEY") == auth.OrgDetails.SecretKey)

	check := func(name string, err error) {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else {
			t.Logf("%s: ok", name)
		}
	}
	st, err := c.States(ctx)
	check("States", err)
	t.Logf("states: %d", len(st))
	if len(st) > 0 {
		_, err = c.Hubs(ctx, st[0].ID)
		check("Hubs", err)
	}
	_, err = c.DeliveryCost(ctx, CostRequest{State: "Kano"})
	check("DeliveryCost", err)
	_, err = c.DeliveryTimeEstimate(ctx, DeliveryTimeEstimateRequest{DeliveryType: "local", PickUpState: "Edo", DropOffState: "Lagos"})
	check("DeliveryTimeEstimate", err)
	_, err = c.ExportLocations(ctx)
	check("ExportLocations", err)
	_, err = c.ImportLocations(ctx)
	check("ImportLocations", err)
	_, err = c.ImportItemCategories(ctx)
	check("ImportItemCategories", err)
	_, err = c.ExportDeliveryCost(ctx, ExportCostRequest{WeightID: 1, ExportLocationID: 1})
	check("ExportDeliveryCost", err)
	_, err = c.ImportDeliveryCost(ctx, ImportCostRequest{DestinationState: "Kano", ImportLocationID: 2, Weight: 5})
	check("ImportDeliveryCost", err)
	_, err = c.Users(ctx)
	check("Users", err)
	_, err = c.SearchOrders(ctx, SearchOrders{StartDate: "2024-01-01", EndDate: time.Now().Format("2006-01-02")})
	check("SearchOrders", err)
	_, err = c.OrderStats(ctx, "2024-01-01", time.Now().Format("2006-01-02"))
	check("OrderStats", err)
	_, err = c.Lockers(ctx, "Lagos")
	check("Lockers", err)
}

// Read-only: proves automatic re-login against the real sandbox by replacing
// the token with a dead one and checking the next call still succeeds.
func TestSandboxAutoRelogin(t *testing.T) {
	loadEnv()
	uid, pw := os.Getenv("FEZ_USER_ID"), os.Getenv("FEZ_PASSWORD")
	if uid == "" || pw == "" {
		t.Skip("FEZ_USER_ID/FEZ_PASSWORD not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := New()
	if _, err := c.Authenticate(ctx, uid, pw); err != nil {
		t.Fatal(err)
	}
	c.mu.RLock()
	good, sk := c.token, c.secretKey
	c.mu.RUnlock()
	c.SetCredentials("EXPIRED-"+good, sk)
	if _, err := c.Users(ctx); err != nil {
		t.Fatalf("call with dead token was not recovered: %v", err)
	}
	c.mu.RLock()
	renewed := c.token
	c.mu.RUnlock()
	if renewed == "" || renewed == "EXPIRED-"+good {
		t.Fatalf("token not renewed: %q", renewed)
	}
	t.Log("dead token rejected, client re-logged in and retried")

	// Lazy login via WithAutoLogin, no Authenticate call.
	if _, err := New(WithAutoLogin(uid, pw)).Users(ctx); err != nil {
		t.Fatalf("WithAutoLogin lazy login: %v", err)
	}

	// Opt-out still surfaces the 401.
	c2 := New(WithoutAutoLogin())
	if _, err := c2.Authenticate(ctx, uid, pw); err != nil {
		t.Fatal(err)
	}
	c2.SetCredentials("EXPIRED-"+good, sk)
	var ae *APIError
	if _, err := c2.Users(ctx); !errors.As(err, &ae) || ae.HTTPStatus != 401 {
		t.Fatalf("WithoutAutoLogin: want 401, got %v", err)
	}
}
