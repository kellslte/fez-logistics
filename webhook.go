package fez

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

// Sign computes the X-Signature value: HMAC-SHA256 (key = your org secret key)
// over orderNo + orderStatus + timestamp, hex encoded.
// The docs say "SHA256 ... signed with the secret key"; if your deliveries fail
// verification, compare against a captured request and adjust here.
func Sign(secretKey, orderNo, status, timestamp string) string {
	m := hmac.New(sha256.New, []byte(secretKey))
	m.Write([]byte(orderNo + status + timestamp))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyWebhook checks signature and timestamp freshness (maxAge<=0 skips age check).
func VerifyWebhook(secretKey string, ev WebhookEvent, timestamp, signature string, maxAge time.Duration) error {
	if maxAge > 0 {
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return errors.New("fez: bad X-Timestamp")
		}
		if d := time.Since(time.Unix(ts, 0)); d > maxAge || d < -maxAge {
			return errors.New("fez: stale webhook timestamp")
		}
	}
	want := Sign(secretKey, ev.OrderNumber, ev.Status, timestamp)
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(want)) {
		return errors.New("fez: invalid webhook signature")
	}
	return nil
}

// WebhookHandler returns an http.Handler that verifies Fez's signature (5 min
// tolerance) and calls fn. It replies 200 on success, 401 on bad signature.
func WebhookHandler(secretKey string, fn func(WebhookEvent)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev WebhookEvent
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&ev); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if err := VerifyWebhook(secretKey, ev, r.Header.Get("X-Timestamp"), r.Header.Get("X-Signature"), 5*time.Minute); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		fn(ev)
		w.WriteHeader(http.StatusOK)
	})
}
