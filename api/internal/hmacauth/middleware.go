package hmacauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	headerSignature = "X-Request-Signature"
	headerTimestamp = "X-Request-Timestamp"
)

var (
	ErrMissingSignature = errors.New("missing request signature")
	ErrMissingTimestamp = errors.New("missing request timestamp")
	ErrStaleTimestamp   = errors.New("stale request timestamp")
	ErrInvalidSignature = errors.New("invalid request signature")
)

type Verifier struct {
	Secret   string
	MaxSkew  time.Duration
	Now      func() time.Time
	BodyCopy bool
}

func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := v.verify(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (v *Verifier) verify(r *http.Request) error {
	if v.Secret == "" {
		return nil
	}

	sig := r.Header.Get(headerSignature)
	if sig == "" {
		return ErrMissingSignature
	}
	tsHeader := r.Header.Get(headerTimestamp)
	if tsHeader == "" {
		return ErrMissingTimestamp
	}
	ts, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		return ErrMissingTimestamp
	}

	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}

	reqTime := time.Unix(ts, 0)
	if now.Sub(reqTime) > v.MaxSkew || reqTime.Sub(now) > v.MaxSkew {
		return ErrStaleTimestamp
	}

	bodyBytes, err := readBody(r)
	if err != nil {
		return err
	}

	expected := computeSignature(v.Secret, tsHeader, bodyBytes)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return ErrInvalidSignature
	}
	return nil
}

func computeSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	return strings.ToLower(hex.EncodeToString(mac.Sum(nil)))
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return []byte{}, nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, nil
}
