package idempotency

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreLifecycle(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	key := "test-key"
	rec := Record{
		StatusCode: 201,
		Response:   []byte("payload"),
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().Add(time.Minute).UTC(),
	}

	if err := store.Save(ctx, key, rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.StatusCode != rec.StatusCode {
		t.Fatalf("unexpected record: %#v", got)
	}
}
