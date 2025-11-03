package idempotency

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists records in a PostgreSQL table.
type PostgresStore struct {
	pool *pgxpool.Pool
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS idempotency_records (
    key TEXT PRIMARY KEY,
    status_code INT NOT NULL,
    response BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
`

// NewPostgresStore connects to Postgres using the DSN and ensures the table exists.
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if dsn == "" {
		return nil, errors.New("postgres dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		pool.Close()
		return nil, err
	}

	return &PostgresStore{pool: pool}, nil
}

func (p *PostgresStore) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

func (p *PostgresStore) Get(ctx context.Context, key string) (*Record, error) {
	row := p.pool.QueryRow(ctx, `
SELECT status_code, response, created_at, expires_at
FROM idempotency_records
WHERE key = $1
`, key)

	var rec Record
	if err := row.Scan(&rec.StatusCode, &rec.Response, &rec.CreatedAt, &rec.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if time.Now().After(rec.ExpiresAt) {
		go p.deleteKey(context.Background(), key)
		return nil, nil
	}
	return &rec, nil
}

func (p *PostgresStore) Save(ctx context.Context, key string, record Record) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO idempotency_records (key, status_code, response, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (key) DO UPDATE
SET status_code = EXCLUDED.status_code,
    response = EXCLUDED.response,
    created_at = EXCLUDED.created_at,
    expires_at = EXCLUDED.expires_at
`, key, record.StatusCode, record.Response, record.CreatedAt, record.ExpiresAt)
	return err
}

func (p *PostgresStore) deleteKey(ctx context.Context, key string) {
	_, _ = p.pool.Exec(ctx, `DELETE FROM idempotency_records WHERE key = $1`, key)
}
