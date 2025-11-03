package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fiatrails/internal/config"
	"fiatrails/internal/escrow"
	"fiatrails/internal/hmacauth"
	"fiatrails/internal/idempotency"
)

type Server struct {
	cfg        *config.AppConfig
	escrow     escrow.Client
	store      idempotency.Store
	hmac       *hmacauth.Verifier
	mpesaHMAC  *hmacauth.Verifier
	httpServer *http.Server
}

func NewServer(cfg *config.AppConfig, esc escrow.Client, store idempotency.Store) *Server {
	hmacVerifier := &hmacauth.Verifier{
		Secret:  cfg.Seed.Secrets.HMACSalt,
		MaxSkew: cfg.Service.HMACClockSkew,
	}

	mpesaVerifier := &hmacauth.Verifier{
		Secret:          cfg.Seed.Secrets.MpesaWebhookSecret,
		MaxSkew:         cfg.Service.HMACClockSkew,
		SignatureHeader: "X-Mpesa-Signature",
		TimestampHeader: "X-Request-Timestamp",
	}

	s := &Server{
		cfg:       cfg,
		escrow:    esc,
		store:     store,
		hmac:      hmacVerifier,
		mpesaHMAC: mpesaVerifier,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/mint-intents", s.hmac.Middleware(http.HandlerFunc(s.handleMintIntents)))
	mux.Handle("/api/v1/callbacks/mpesa", s.mpesaHMAC.Middleware(http.HandlerFunc(s.handleMpesaCallback)))

	s.httpServer = &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Service.HTTPPort),
		Handler:           requestIDMiddleware(mux),
		ReadHeaderTimeout: 15 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	log.Printf("API listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

type mintIntentRequest struct {
	UserAddress string `json:"userAddress"`
	Amount      string `json:"amount"`
	CountryCode string `json:"countryCode"`
	TxRef       string `json:"txRef"`
}

type mintIntentResponse struct {
	IntentID string `json:"intentId"`
	Status   string `json:"status"`
	TxHash   string `json:"txHash,omitempty"`
}

type mpesaCallbackRequest struct {
	IntentID    string `json:"intentId"`
	TxRef       string `json:"txRef"`
	UserAddress string `json:"userAddress"`
	Amount      string `json:"amount"`
}

type mpesaCallbackResponse struct {
	Status   string `json:"status"`
	IntentID string `json:"intentId"`
	TxHash   string `json:"txHash,omitempty"`
}

const mpesaKeyPrefix = "mpesa:"

func (s *Server) handleMintIntents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
	if key == "" {
		http.Error(w, "missing X-Idempotency-Key header", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if existing, _ := s.store.Get(ctx, key); existing != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(existing.StatusCode)
		_, _ = w.Write(existing.Response)
		return
	}

	var payload mintIntentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}
	if err := validateMintIntentRequest(payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := s.escrow.SubmitIntent(ctx, escrow.SubmitIntentRequest{
		UserAddress: payload.UserAddress,
		Amount:      payload.Amount,
		CountryCode: payload.CountryCode,
		TxRef:       payload.TxRef,
	})
	if err != nil {
		http.Error(w, "failed to submit intent: "+err.Error(), http.StatusBadGateway)
		return
	}

	respBody := mintIntentResponse{
		IntentID: result.IntentID,
		Status:   "submitted",
		TxHash:   result.TxHash,
	}
	b, _ := json.Marshal(respBody)

	record := idempotency.Record{
		StatusCode: http.StatusCreated,
		Response:   b,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(s.cfg.Service.IdempotencyWindow),
	}
	_ = s.store.Save(ctx, key, record)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(b)
}

func (s *Server) handleMpesaCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	var payload mpesaCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}
	if err := validateMpesaRequest(payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	key := mpesaKeyPrefix + payload.TxRef
	if existing, _ := s.store.Get(ctx, key); existing != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(existing.StatusCode)
		_, _ = w.Write(existing.Response)
		return
	}

	txHash, err := s.executeMintWithRetry(ctx, payload.IntentID)
	if err != nil {
		s.writeDLQ(payload, err)
		http.Error(w, "failed to execute mint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := mpesaCallbackResponse{
		Status:   "processed",
		IntentID: payload.IntentID,
		TxHash:   txHash,
	}
	body, _ := json.Marshal(resp)

	record := idempotency.Record{
		StatusCode: http.StatusOK,
		Response:   body,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(s.cfg.Service.IdempotencyWindow),
	}
	_ = s.store.Save(ctx, key, record)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func validateMintIntentRequest(req mintIntentRequest) error {
	if req.UserAddress == "" {
		return errors.New("userAddress is required")
	}
	if req.Amount == "" {
		return errors.New("amount is required")
	}
	if req.CountryCode == "" {
		return errors.New("countryCode is required")
	}
	if req.TxRef == "" {
		return errors.New("txRef is required")
	}
	return nil
}

func validateMpesaRequest(req mpesaCallbackRequest) error {
	if req.IntentID == "" {
		return errors.New("intentId is required")
	}
	if req.TxRef == "" {
		return errors.New("txRef is required")
	}
	if req.UserAddress == "" {
		return errors.New("userAddress is required")
	}
	if req.Amount == "" {
		return errors.New("amount is required")
	}
	return nil
}

func (s *Server) executeMintWithRetry(ctx context.Context, intentID string) (string, error) {
	attempts := s.cfg.Retry.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	backoff := s.cfg.Retry.InitialBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}

	for i := 1; i <= attempts; i++ {
		resp, err := s.escrow.ExecuteMint(ctx, intentID)
		if err == nil {
			return resp.TxHash, nil
		}
		if !isRetryable(err) || i == attempts {
			return "", err
		}

		sleep := backoff
		if s.cfg.Retry.MaxBackoff > 0 && sleep > s.cfg.Retry.MaxBackoff {
			sleep = s.cfg.Retry.MaxBackoff
		}
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return "", ctx.Err()
		}

		if s.cfg.Retry.BackoffMultiplier > 1 {
			backoff = backoff * time.Duration(s.cfg.Retry.BackoffMultiplier)
		}
	}

	return "", fmt.Errorf("exhausted retries")
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "UserNotCompliant") {
		return false
	}
	if strings.Contains(strings.ToLower(msg), "invalid") {
		return false
	}
	return true
}

func (s *Server) writeDLQ(payload mpesaCallbackRequest, execErr error) {
	if s.cfg.Service.DLQPath == "" {
		return
	}

	entry := struct {
		Timestamp time.Time            `json:"timestamp"`
		Payload   mpesaCallbackRequest `json:"payload"`
		Error     string               `json:"error"`
	}{
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		Error:     execErr.Error(),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		log.Printf("dlq marshal error: %v", err)
		return
	}

	if err := os.MkdirAll(s.cfg.Service.DLQPath, 0o755); err != nil {
		log.Printf("dlq mkdir error: %v", err)
		return
	}

	filename := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), payload.TxRef)
	path := filepath.Join(s.cfg.Service.DLQPath, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("dlq write error: %v", err)
	}
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-Id") == "" {
			r.Header.Set("X-Request-Id", fmt.Sprintf("%d", time.Now().UnixNano()))
		}
		next.ServeHTTP(w, r)
	})
}
