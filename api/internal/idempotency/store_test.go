package idempotency

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if rec, _ := store.Get(ctx, "missing"); rec != nil {
		t.Fatalf("expected nil for missing key")
	}

	record := Record{
		StatusCode: 201,
		Response:   []byte("ok"),
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Minute),
	}
	if err := store.Save(ctx, "abc", record); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, _ := store.Get(ctx, "abc")
	if got == nil || string(got.Response) != "ok" {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestFileStorePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idem.json")

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	ctx := context.Background()
	record := Record{
		StatusCode: 201,
		Response:   []byte("resp"),
		CreatedAt:  time.Unix(0, 0),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	if err := store.Save(ctx, "key", record); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}

	store2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}

	got, _ := store2.Get(ctx, "key")
	if got == nil || string(got.Response) != "resp" {
		t.Fatalf("unexpected record: %+v", got)
	}
}
