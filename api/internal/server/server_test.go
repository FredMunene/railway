package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
				HMACSalt:           "test-secret",
				MpesaWebhookSecret: "mpesa-secret",
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
			DLQPath:           t.TempDir(),
		},
		Retry: config.RetryConfig{
			MaxAttempts:       1,
			InitialBackoff:    time.Millisecond,
			MaxBackoff:        time.Millisecond,
			BackoffMultiplier: 1,
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

func TestMpesaCallbackIdempotency(t *testing.T) {
	cfg := &config.AppConfig{
		Seed: config.SeedConfig{
			Secrets: struct {
				HMACSalt           string `json:"hmacSalt"`
				IdempotencyKeySalt string `json:"idempotencyKeySalt"`
				MpesaWebhookSecret string `json:"mpesaWebhookSecret"`
			}{
				HMACSalt:           "mint-secret",
				MpesaWebhookSecret: "mpesa-secret",
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
			DLQPath:           t.TempDir(),
		},
		Retry: config.RetryConfig{
			MaxAttempts:       2,
			InitialBackoff:    time.Millisecond,
			MaxBackoff:        2 * time.Millisecond,
			BackoffMultiplier: 2,
		},
	}

	store := idempotency.NewMemoryStore()
	esc := &stubEscrow{executeHashes: []string{"0xdeadbeef"}}
	srv := NewServer(cfg, esc, store)

	payload := mpesaCallbackRequest{
		IntentID:    "0xabc1230000000000000000000000000000000000000000000000000000000000",
		TxRef:       "mpesa-1",
		UserAddress: "0xabc",
		Amount:      "1000000000000000000",
	}
	body, _ := json.Marshal(payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeSignatureForTest(cfg.Seed.Secrets.MpesaWebhookSecret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/mpesa", bytes.NewReader(body))
	req.Header.Set("X-Mpesa-Signature", sig)
	req.Header.Set("X-Request-Timestamp", ts)
	rec := httptest.NewRecorder()

	srv.mpesaHMAC.Middleware(http.HandlerFunc(srv.handleMpesaCallback)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}

	first := rec.Body.Bytes()

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/mpesa", bytes.NewReader(body))
	req2.Header.Set("X-Mpesa-Signature", sig)
	req2.Header.Set("X-Request-Timestamp", ts)
	rec2 := httptest.NewRecorder()
	srv.mpesaHMAC.Middleware(http.HandlerFunc(srv.handleMpesaCallback)).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec2.Code)
	}
	if !bytes.Equal(first, rec2.Body.Bytes()) {
		t.Fatalf("expected same response body on idempotent callback")
	}
	if esc.executeCalls != 1 {
		t.Fatalf("expected execute mint called once, got %d", esc.executeCalls)
	}
}

func TestMpesaCallbackDLQOnFailure(t *testing.T) {
	dlqDir := t.TempDir()
	cfg := &config.AppConfig{
		Seed: config.SeedConfig{
			Secrets: struct {
				HMACSalt           string `json:"hmacSalt"`
				IdempotencyKeySalt string `json:"idempotencyKeySalt"`
				MpesaWebhookSecret string `json:"mpesaWebhookSecret"`
			}{
				HMACSalt:           "mint-secret",
				MpesaWebhookSecret: "mpesa-secret",
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
			DLQPath:           dlqDir,
		},
		Retry: config.RetryConfig{
			MaxAttempts:       2,
			InitialBackoff:    time.Millisecond,
			MaxBackoff:        2 * time.Millisecond,
			BackoffMultiplier: 2,
		},
	}

	failing := &stubEscrow{executeErrs: []error{errors.New("network error"), errors.New("network error")}}
	store := idempotency.NewMemoryStore()
	srv := NewServer(cfg, failing, store)

	payload := mpesaCallbackRequest{
		IntentID:    "0xdef4560000000000000000000000000000000000000000000000000000000000",
		TxRef:       "mpesa-2",
		UserAddress: "0xdef",
		Amount:      "100",
	}
	body, _ := json.Marshal(payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeSignatureForTest(cfg.Seed.Secrets.MpesaWebhookSecret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/mpesa", bytes.NewReader(body))
	req.Header.Set("X-Mpesa-Signature", sig)
	req.Header.Set("X-Request-Timestamp", ts)
	rec := httptest.NewRecorder()

	srv.mpesaHMAC.Middleware(http.HandlerFunc(srv.handleMpesaCallback)).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d", rec.Code)
	}

	entries, err := os.ReadDir(dlqDir)
	if err != nil {
		t.Fatalf("dlq dir read: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected dlq entry")
	}
}

type stubEscrow struct {
	executeHashes []string
	executeErrs   []error
	executeCalls  int
}

func (s *stubEscrow) SubmitIntent(context.Context, escrow.SubmitIntentRequest) (escrow.SubmitIntentResponse, error) {
	return escrow.SubmitIntentResponse{}, nil
}

func (s *stubEscrow) ExecuteMint(context.Context, string) (escrow.ExecuteMintResponse, error) {
	idx := s.executeCalls
	s.executeCalls++
	if idx < len(s.executeErrs) && s.executeErrs[idx] != nil {
		return escrow.ExecuteMintResponse{}, s.executeErrs[idx]
	}
	hash := "0xhash"
	if idx < len(s.executeHashes) && s.executeHashes[idx] != "" {
		hash = s.executeHashes[idx]
	}
	return escrow.ExecuteMintResponse{TxHash: hash}, nil
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
