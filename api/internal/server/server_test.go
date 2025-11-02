package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"fiatrails/internal/config"
	"fiatrails/internal/escrow"
	"fiatrails/internal/idempotency"
)

func TestMintIntentIdempotency(t *testing.T) {
	cfg := &config.AppConfig{
		Seed: config.SeedConfig{
			Secrets: struct {
				HMACSalt           string `json:"hmacSalt"`
				IdempotencyKeySalt string `json:"idempotencyKeySalt"`
				MpesaWebhookSecret string `json:"mpesaWebhookSecret"`
			}{
				HMACSalt: "test-secret",
			},
			Timeouts: struct {
				RPCTimeoutMs          int `json:"rpcTimeoutMs"`
				WebhookTimeoutMs      int `json:"webhookTimeoutMs"`
				IdempotencyWindowSecs int `json:"idempotencyWindowSeconds"`
			}{
				IdempotencyWindowSecs: 60,
			},
		},
		Service: config.ServiceConfig{
			HTTPPort:          0,
			HMACClockSkew:     time.Minute,
			IdempotencyWindow: time.Minute,
		},
	}

	store := idempotency.NewMemoryStore()
	srv := NewServer(cfg, escrow.FakeClient{}, store)

	body := map[string]string{
		"userAddress": "0xabc",
		"amount":      "1000000000000000000",
		"countryCode": "KES",
		"txRef":       "tx-1",
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mint-intents", bytes.NewReader(payload))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-Request-Timestamp", ts)
	req.Header.Set("X-Request-Signature", computeSignatureForTest(cfg.Seed.Secrets.HMACSalt, ts, payload))
	req.Header.Set("X-Idempotency-Key", "key-1")

	rec := httptest.NewRecorder()
	srv.hmac.Middleware(http.HandlerFunc(srv.handleMintIntents)).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d", rec.Code)
	}

	firstPayload := rec.Body.Bytes()

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/mint-intents", bytes.NewReader(payload))
	req2.Header.Set("X-Request-Timestamp", ts)
	req2.Header.Set("X-Request-Signature", computeSignatureForTest(cfg.Seed.Secrets.HMACSalt, ts, payload))
	req2.Header.Set("X-Idempotency-Key", "key-1")
	rec2 := httptest.NewRecorder()
	srv.hmac.Middleware(http.HandlerFunc(srv.handleMintIntents)).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected cached 201 got %d", rec2.Code)
	}
	if !bytes.Equal(firstPayload, rec2.Body.Bytes()) {
		t.Fatalf("expected same response body on idempotent request")
	}
}

func computeSignatureForTest(secret, timestamp string, body []byte) string {
	h := sha256Sum(secret, timestamp, body)
	return hex.EncodeToString(h)
}

func sha256Sum(secret, timestamp string, body []byte) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(timestamp))
	h.Write(body)
	return h.Sum(nil)
}
