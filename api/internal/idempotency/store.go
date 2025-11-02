package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record holds stored response data.
type Record struct {
	StatusCode int       `json:"statusCode"`
	Response   []byte    `json:"response"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// Store abstracts idempotency persistence.
type Store interface {
	Get(ctx context.Context, key string) (*Record, error)
	Save(ctx context.Context, key string, record Record) error
}

// MemoryStore is mostly for testing.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]Record),
	}
}

func (m *MemoryStore) Get(_ context.Context, key string) (*Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(rec.ExpiresAt) {
		return nil, nil
	}
	return &rec, nil
}

func (m *MemoryStore) Save(_ context.Context, key string, record Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = record
	return nil
}

// FileStore persists records to disk. Suitable for local dev; can be swapped with SQLite later.
type FileStore struct {
	path string
	mu   sync.Mutex
	data map[string]Record
}

func NewFileStore(path string) (*FileStore, error) {
	fs := &FileStore{
		path: path,
		data: make(map[string]Record),
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (f *FileStore) load() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	blob, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(blob) == 0 {
		return nil
	}
	return json.Unmarshal(blob, &f.data)
}

func (f *FileStore) persist() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(f.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, blob, 0o600)
}

func (f *FileStore) Get(_ context.Context, key string) (*Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.data[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(record.ExpiresAt) {
		delete(f.data, key)
		_ = f.persist()
		return nil, nil
	}
	return &record, nil
}

func (f *FileStore) Save(_ context.Context, key string, record Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = record
	return f.persist()
}
