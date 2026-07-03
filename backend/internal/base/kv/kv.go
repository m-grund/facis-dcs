package kv

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store provides a generic key-value store backed by Postgres.
type Store struct {
	db *sqlx.DB
}

// NewStore creates a new KV store backed by the given database.
func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

// Get retrieves a value by key. Returns (value, found=true, nil error) on hit;
// (empty string, found=false, nil error) on miss or expiry;
// (empty string, found=false, error) on database error.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	var expiresAt *time.Time
	err := s.db.QueryRowContext(
		ctx,
		"SELECT value, expires_at FROM kv_store WHERE key = $1 AND (expires_at IS NULL OR expires_at > now())",
		key,
	).Scan(&value, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

// Set stores a key-value pair with optional TTL. A TTL <= 0 results in no expiry.
// Returns an error only on database failure.
func (s *Store) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO kv_store (key, value, expires_at, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (key) DO UPDATE
		 SET value = $2, expires_at = $3, updated_at = now()`,
		key, value, expiresAt,
	)
	return err
}
