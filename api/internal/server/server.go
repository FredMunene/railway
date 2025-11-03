package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	httpServer *http.Server
}

func NewServer(cfg *config.AppConfig, esc escrow.Client, store idempotency.Store) *Server {
	hmacVerifier := &hmacauth.Verifier{
		Secret:  cfg.Seed.Secrets.HMACSalt,
		MaxSkew: cfg.Service.HMACClockSkew,
	}

	s := &Server{
		cfg:    cfg,
		escrow: esc,
		store:  store,
		hmac:   hmacVerifier,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/mint-intents", s.hmac.Middleware(http.HandlerFunc(s.handleMintIntents)))

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

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-Id") == "" {
			r.Header.Set("X-Request-Id", fmt.Sprintf("%d", time.Now().UnixNano()))
		}
		next.ServeHTTP(w, r)
	})
}
