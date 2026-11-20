package fez

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestAuthThenCreateOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/authenticate":
			w.Write([]byte(`{"status":"Success","authDetails":{"authToken":"tok","expireToken":"x"},"orgDetails":{"secret-key":"sk"}}`))
		case "/order":
			if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("secret-key") != "sk" {
				w.WriteHeader(401)
				w.Write([]byte(`{"status":"Error","description":"Organization Secret Key is Required"}`))
				return
			}
			var in []CreateOrder
			json.NewDecoder(r.Body).Decode(&in)
			w.WriteHeader(201)
			w.Write([]byte(`{"status":"Success","orderNos":{"` + in[0].UniqueID + `":"ABC"}}`))
		}
	}))
	defer srv.Close()
	c := New(WithBaseURL(srv.URL))
	ctx := context.Background()
	if _, err := c.CreateOrders(ctx, []CreateOrder{{UniqueID: "u1"}}); err == nil {
		t.Fatal("expected 401 before auth")
	}
	if _, err := c.Authenticate(ctx, "u", "p"); err != nil {
		t.Fatal(err)
	}
	r, err := c.CreateOrders(ctx, []CreateOrder{{UniqueID: "u1"}})
	if err != nil || r.OrderNos["u1"] != "ABC" {
		t.Fatal(r, err)
	}
}

func TestWebhookVerify(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	ev := WebhookEvent{OrderNumber: "O1", Status: "Delivered"}
	sig := Sign("sk", "O1", "Delivered", ts)
	if err := VerifyWebhook("sk", ev, ts, sig, time.Minute); err != nil {
		t.Fatal(err)
	}
	if VerifyWebhook("sk", ev, ts, "bad", time.Minute) == nil {
		t.Fatal("bad sig accepted")
	}
}

func TestWebhookEmptyKeyRejected(t *testing.T) {
	ev := WebhookEvent{OrderNumber: "O1", Status: "Delivered"}
	if VerifyWebhook("", ev, "1", Sign("", "O1", "Delivered", "1"), 0) == nil {
		t.Fatal("empty key accepted")
	}
}
