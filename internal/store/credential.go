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

// Credential status constants.
const (
	CredentialStatusActive  = "active"
	CredentialStatusDeleted = "deleted"
)

// BankCredential represents an encrypted bank credential record.
type BankCredential struct {
	ID             uuid.UUID
	BankCode       string
	AccountLabel   string
	CredentialsEnc []byte
	CredentialsDEK []byte
	Version        int
	Status         string
	DeletedAt      *time.Time
	CreatedBy      uuid.UUID
	UpdatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CredentialRepository defines operations on the bank_credentials table.
type CredentialRepository interface {
	Create(ctx context.Context, c *BankCredential) error
	GetByID(ctx context.Context, id uuid.UUID) (*BankCredential, error)
	GetActiveByBankCode(ctx context.Context, bankCode string) (*BankCredential, error)
	List(ctx context.Context) ([]BankCredential, error)
	Update(ctx context.Context, c *BankCredential) error
	SoftDelete(ctx context.Context, id, deletedBy uuid.UUID) error
	HardDeleteExpired(ctx context.Context, retentionDays int) (int64, error)
}

// CredentialRepo implements CredentialRepository using pgx.
type CredentialRepo struct {
	pool *pgxpool.Pool
}

// NewCredentialRepo creates a new CredentialRepo.
func NewCredentialRepo(pool *pgxpool.Pool) *CredentialRepo {
	return &CredentialRepo{pool: pool}
}

// Create inserts a new bank credential and populates its generated fields.
func (r *CredentialRepo) Create(ctx context.Context, c *BankCredential) error {
	query := `
		INSERT INTO bank_credentials (bank_code, account_label, credentials_enc, credentials_dek, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, version, status, deleted_at, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		c.BankCode, c.AccountLabel, c.CredentialsEnc, c.CredentialsDEK, c.CreatedBy, c.UpdatedBy,
	).Scan(&c.ID, &c.Version, &c.Status, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

const credentialColumns = `id, bank_code, account_label, credentials_enc, credentials_dek,
	version, status, deleted_at, created_by, updated_by, created_at, updated_at`

func scanCredential(row pgx.Row) (*BankCredential, error) {
	c := &BankCredential{}
	err := row.Scan(
		&c.ID, &c.BankCode, &c.AccountLabel, &c.CredentialsEnc, &c.CredentialsDEK,
		&c.Version, &c.Status, &c.DeletedAt, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// GetByID retrieves a bank credential by its UUID.
func (r *CredentialRepo) GetByID(ctx context.Context, id uuid.UUID) (*BankCredential, error) {
	query := `SELECT ` + credentialColumns + ` FROM bank_credentials WHERE id = $1`

	c, err := scanCredential(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("credential %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get credential by id: %w", err)
	}
	return c, nil
}

// GetActiveByBankCode retrieves the active credential for a given bank code.
func (r *CredentialRepo) GetActiveByBankCode(ctx context.Context, bankCode string) (*BankCredential, error) {
	query := `SELECT ` + credentialColumns + ` FROM bank_credentials WHERE bank_code = $1 AND status = $2`

	c, err := scanCredential(r.pool.QueryRow(ctx, query, bankCode, CredentialStatusActive))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("credential for %s: %w", bankCode, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get credential by bank code: %w", err)
	}
	return c, nil
}

// List returns all active bank credentials ordered by creation date descending.
func (r *CredentialRepo) List(ctx context.Context) ([]BankCredential, error) {
	query := `SELECT ` + credentialColumns + ` FROM bank_credentials WHERE status = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, CredentialStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	var creds []BankCredential
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		creds = append(creds, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credentials: %w", err)
	}

	return creds, nil
}

// Update modifies an active credential and increments its version.
func (r *CredentialRepo) Update(ctx context.Context, c *BankCredential) error {
	query := `
		UPDATE bank_credentials
		SET bank_code = $2, account_label = $3, credentials_enc = $4, credentials_dek = $5,
		    updated_by = $6, version = version + 1, updated_at = now()
		WHERE id = $1 AND status = $7
		RETURNING version, updated_at`

	err := r.pool.QueryRow(ctx, query,
		c.ID, c.BankCode, c.AccountLabel, c.CredentialsEnc, c.CredentialsDEK, c.UpdatedBy, CredentialStatusActive,
	).Scan(&c.Version, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("credential %s: %w", c.ID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	return nil
}

// SoftDelete marks an active credential as deleted without removing its row.
func (r *CredentialRepo) SoftDelete(ctx context.Context, id, deletedBy uuid.UUID) error {
	query := `
		UPDATE bank_credentials
		SET status = $3, deleted_at = now(), updated_by = $2, updated_at = now()
		WHERE id = $1 AND status = $4`

	tag, err := r.pool.Exec(ctx, query, id, deletedBy, CredentialStatusDeleted, CredentialStatusActive)
	if err != nil {
		return fmt.Errorf("soft delete credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("credential %s: %w", id, ErrNotFound)
	}
	return nil
}

// HardDeleteExpired permanently removes soft-deleted credentials older than retentionDays.
func (r *CredentialRepo) HardDeleteExpired(ctx context.Context, retentionDays int) (int64, error) {
	query := `DELETE FROM bank_credentials WHERE status = $1 AND deleted_at < now() - make_interval(days => $2)`

	tag, err := r.pool.Exec(ctx, query, CredentialStatusDeleted, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("hard delete expired credentials: %w", err)
	}
	return tag.RowsAffected(), nil
}
