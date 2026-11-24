// Command webhook-capture is a local receiver for Fez webhooks. It logs every
// request (headers + raw body) to stdout and captures.jsonl, and tests several
// candidate signature schemes against X-Signature so you can see which one Fez
// really uses. Always replies 200. Sandbox/dev tool only.
//
//	FEZ_SECRET_KEY=... go run ./examples/webhook-capture   # listens on :8787
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func h(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func hm(key string, msg string) []byte {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func main() {
	secret := os.Getenv("FEZ_SECRET_KEY")
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	out, err := os.OpenFile("captures.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })
	http.HandleFunc("/hooks/fez", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var ev struct {
			OrderNumber string `json:"orderNumber"`
			Status      string `json:"status"`
			OrderStatus string `json:"orderStatus"`
		}
		json.Unmarshal(body, &ev)
		st := ev.Status
		if st == "" {
			st = ev.OrderStatus
		}
		ts, sig := r.Header.Get("X-Timestamp"), strings.ToLower(r.Header.Get("X-Signature"))

		cands := map[string]string{}
		if secret != "" {
			cands["HMAC-SHA256(secret, orderNo+status+ts) hex"] = hex.EncodeToString(hm(secret, ev.OrderNumber+st+ts))
			cands["HMAC-SHA256(secret, orderNo+status+ts) base64"] = base64.StdEncoding.EncodeToString(hm(secret, ev.OrderNumber+st+ts))
			cands["HMAC-SHA256(secret, ts+orderNo+status) hex"] = hex.EncodeToString(hm(secret, ts+ev.OrderNumber+st))
			cands["HMAC-SHA256(secret, rawBody) hex"] = hex.EncodeToString(hm(secret, string(body)))
			cands["HMAC-SHA256(secret, ts+rawBody) hex"] = hex.EncodeToString(hm(secret, ts+string(body)))
			cands["HMAC-SHA256(secret, ts.rawBody) hex"] = hex.EncodeToString(hm(secret, ts+"."+string(body)))
			cands["SHA256(orderNo+status+ts+secret) hex"] = h([]byte(ev.OrderNumber + st + ts + secret))
			cands["SHA256(secret+orderNo+status+ts) hex"] = h([]byte(secret + ev.OrderNumber + st + ts))
			cands["SHA256(orderNo+status+ts) hex (unkeyed)"] = h([]byte(ev.OrderNumber + st + ts))
		}
		var matched []string
		for name, v := range cands {
			if strings.EqualFold(v, sig) {
				matched = append(matched, name)
			}
		}
		hdr := map[string]string{}
		for k, v := range r.Header {
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "secret-key") {
				continue // never log credentials
			}
			hdr[k] = strings.Join(v, ",")
		}
		rec := map[string]any{"time": time.Now().Format(time.RFC3339), "method": r.Method, "path": r.URL.String(), "headers": hdr, "body": string(body), "signatureMatches": matched}
		b, _ := json.Marshal(rec)
		out.Write(append(b, '\n'))
		b, _ = json.MarshalIndent(rec, "", "  ")
		log.Printf("webhook received:\n%s", b)
		if len(matched) == 0 {
			log.Printf("NO signature scheme matched (secret set: %v)", secret != "")
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"received":true}`)
	})
	log.Printf("listening on http://%s  (webhook path: /hooks/fez)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
