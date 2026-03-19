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

// Session represents an authenticated credential manager session.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	IPAddress  string
	UserAgent  string
	ExpiresAt  time.Time
	LastActive time.Time
	CreatedAt  time.Time
}

// SessionRepository defines operations on the sessions table.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	GetByTokenHash(ctx context.Context, hash string) (*Session, error)
	TouchLastActive(ctx context.Context, id uuid.UUID, now time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

// SessionRepo implements SessionRepository using pgx.
type SessionRepo struct {
	pool *pgxpool.Pool
}

// NewSessionRepo creates a new SessionRepo.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{pool: pool}
}

func (r *SessionRepo) Create(ctx context.Context, s *Session) error {
	query := `
		INSERT INTO sessions (user_id, token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, last_active, created_at`

	return r.pool.QueryRow(ctx, query,
		s.UserID, s.TokenHash, s.IPAddress, s.UserAgent, s.ExpiresAt,
	).Scan(&s.ID, &s.LastActive, &s.CreatedAt)
}

func (r *SessionRepo) GetByTokenHash(ctx context.Context, hash string) (*Session, error) {
	query := `
		SELECT id, user_id, token_hash, host(ip_address), user_agent, expires_at, last_active, created_at
		FROM sessions WHERE token_hash = $1`

	s := &Session{}
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent,
		&s.ExpiresAt, &s.LastActive, &s.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("session: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session by token hash: %w", err)
	}
	return s, nil
}

func (r *SessionRepo) TouchLastActive(ctx context.Context, id uuid.UUID, now time.Time) error {
	query := `UPDATE sessions SET last_active = $2 WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id, now)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *SessionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM sessions WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *SessionRepo) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM sessions WHERE expires_at < now()`

	tag, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
