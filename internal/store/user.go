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

// User represents a credential manager admin user.
type User struct {
	ID             uuid.UUID
	Username       string
	PasswordHash   string
	TOTPSecretEnc  []byte
	TOTPSecretDEK  []byte
	IsActive       bool
	FailedAttempts int
	LockedUntil    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserRepository defines operations on the users table.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	IncrementFailedAttempts(ctx context.Context, id uuid.UUID) (int, error)
	ResetFailedAttempts(ctx context.Context, id uuid.UUID) error
	LockUntil(ctx context.Context, id uuid.UUID, until time.Time) error
}

// UserRepo implements UserRepository using pgx.
type UserRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

func (r *UserRepo) Create(ctx context.Context, u *User) error {
	query := `
		INSERT INTO users (username, password_hash, totp_secret_enc, totp_secret_dek)
		VALUES ($1, $2, $3, $4)
		RETURNING id, is_active, failed_attempts, locked_until, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		u.Username, u.PasswordHash, u.TOTPSecretEnc, u.TOTPSecretDEK,
	).Scan(
		&u.ID, &u.IsActive, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

const userColumns = `id, username, password_hash, totp_secret_enc, totp_secret_dek,
	       is_active, failed_attempts, locked_until, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecretEnc, &u.TOTPSecretDEK,
		&u.IsActive, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	u, err := scanUser(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE username = $1`

	u, err := scanUser(r.pool.QueryRow(ctx, query, username))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user %q: %w", username, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func (r *UserRepo) IncrementFailedAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	query := `
		UPDATE users SET failed_attempts = failed_attempts + 1, updated_at = now()
		WHERE id = $1
		RETURNING failed_attempts`

	var count int
	err := r.pool.QueryRow(ctx, query, id).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("user %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("increment failed attempts: %w", err)
	}
	return count, nil
}

func (r *UserRepo) ResetFailedAttempts(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET failed_attempts = 0, locked_until = NULL, updated_at = now() WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("reset failed attempts: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *UserRepo) LockUntil(ctx context.Context, id uuid.UUID, until time.Time) error {
	query := `UPDATE users SET locked_until = $2, updated_at = now() WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id, until)
	if err != nil {
		return fmt.Errorf("lock user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %s: %w", id, ErrNotFound)
	}
	return nil
}
