package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// APIKey represents an API key record.
type APIKey struct {
	ID          uuid.UUID
	KeyHash     []byte
	ClientID    string
	Description *string
	CreatedAt   time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
}

// APIKeyRepository defines operations on the api_keys table.
type APIKeyRepository interface {
	Create(ctx context.Context, k *APIKey) error
	GetByKeyHash(ctx context.Context, keyHash []byte) (*APIKey, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]APIKey, error)
}

// APIKeyRepo implements APIKeyRepository using pgx.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo creates a new APIKeyRepo.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

const apiKeyColumns = `id, key_hash, client_id, description, created_at, revoked_at, last_used_at`

func scanAPIKey(row pgx.Row) (*APIKey, error) {
	k := &APIKey{}
	err := row.Scan(
		&k.ID, &k.KeyHash, &k.ClientID, &k.Description, &k.CreatedAt, &k.RevokedAt, &k.LastUsedAt,
	)
	return k, err
}

// Create inserts a new API key.
func (r *APIKeyRepo) Create(ctx context.Context, k *APIKey) error {
	query := `
		INSERT INTO api_keys (key_hash, client_id, description)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, revoked_at, last_used_at`

	err := r.pool.QueryRow(ctx, query,
		k.KeyHash, k.ClientID, k.Description,
	).Scan(&k.ID, &k.CreatedAt, &k.RevokedAt, &k.LastUsedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// GetByKeyHash looks up an API key by its SHA-256 hash.
func (r *APIKeyRepo) GetByKeyHash(ctx context.Context, keyHash []byte) (*APIKey, error) {
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE key_hash = $1`

	k, err := scanAPIKey(r.pool.QueryRow(ctx, query, keyHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("api key: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return k, nil
}

// Revoke sets revoked_at on an active API key.
func (r *APIKeyRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key %s: %w", id, ErrNotFound)
	}
	return nil
}

// UpdateLastUsed sets last_used_at to now.
func (r *APIKeyRepo) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE api_keys SET last_used_at = now() WHERE id = $1`

	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	return nil
}

// List returns all API keys ordered by creation date.
func (r *APIKeyRepo) List(ctx context.Context) ([]APIKey, error) {
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, *k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}

	return keys, nil
}
