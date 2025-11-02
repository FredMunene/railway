package hmacauth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMiddleware_AllowsValidSignature(t *testing.T) {
	body := `{"hello":"world"}`
	now := time.Unix(1_700_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := computeSignature("secret", ts, []byte(body))

	v := &Verifier{
		Secret:  "secret",
		MaxSkew: time.Minute,
		Now: func() time.Time {
			return now
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set(headerSignature, sig)
	req.Header.Set(headerTimestamp, ts)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	v.Middleware(handler).ServeHTTP(rec, req)

	if !called {
		t.Fatalf("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_RejectsInvalidSignature(t *testing.T) {
	body := `{"foo":"bar"}`
	now := time.Unix(1_700_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)

	v := &Verifier{
		Secret:  "secret",
		MaxSkew: time.Minute,
		Now: func() time.Time {
			return now
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set(headerSignature, "deadbeef")
	req.Header.Set(headerTimestamp, ts)
	rec := httptest.NewRecorder()

	v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
