package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account status constants.
const (
	AccountStatusActive   = "active"
	AccountStatusInactive = "inactive"
)

// Account type constants.
const (
	AccountTypeChecking = "checking"
	AccountTypeSavings  = "savings"
)

// Account represents a discovered bank account.
type Account struct {
	ID            uuid.UUID
	BankCode      string
	AccountNumber string
	Currency      string
	AccountType   string // "checking", "savings"
	Status        string // "active", "inactive"
	CredentialID  uuid.UUID
	LastSyncedAt  *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AccountFilter holds optional filters for listing accounts.
type AccountFilter struct {
	BankCode *string
	Currency *string
}

// AccountRepository defines operations on the accounts table.
type AccountRepository interface {
	Create(ctx context.Context, a *Account) error
	GetByID(ctx context.Context, id uuid.UUID) (*Account, error)
	List(ctx context.Context, filter AccountFilter) ([]Account, error)
	UpsertBatch(ctx context.Context, credentialID uuid.UUID, accounts []Account) error
	UpdateLastSynced(ctx context.Context, id uuid.UUID) error
}

// AccountRepo implements AccountRepository using pgx.
type AccountRepo struct {
	pool *pgxpool.Pool
}

// NewAccountRepo creates a new AccountRepo.
func NewAccountRepo(pool *pgxpool.Pool) *AccountRepo {
	return &AccountRepo{pool: pool}
}

const accountColumns = `id, bank_code, account_number, currency, account_type,
	status, credential_id, last_synced_at, created_at, updated_at`

func scanAccount(row pgx.Row) (*Account, error) {
	a := &Account{}
	err := row.Scan(
		&a.ID, &a.BankCode, &a.AccountNumber, &a.Currency, &a.AccountType,
		&a.Status, &a.CredentialID, &a.LastSyncedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

func (r *AccountRepo) Create(ctx context.Context, a *Account) error {
	query := `
		INSERT INTO accounts (bank_code, account_number, currency, account_type, credential_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, status, last_synced_at, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		a.BankCode, a.AccountNumber, a.Currency, a.AccountType, a.CredentialID,
	).Scan(&a.ID, &a.Status, &a.LastSyncedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (r *AccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	query := `SELECT ` + accountColumns + ` FROM accounts WHERE id = $1`

	a, err := scanAccount(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("account %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return a, nil
}

func (r *AccountRepo) List(ctx context.Context, filter AccountFilter) ([]Account, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if filter.BankCode != nil {
		conditions = append(conditions, fmt.Sprintf("bank_code = $%d", argIdx))
		args = append(args, *filter.BankCode)
		argIdx++
	}
	if filter.Currency != nil {
		conditions = append(conditions, fmt.Sprintf("currency = $%d", argIdx))
		args = append(args, *filter.Currency)
		argIdx++
	}

	query := `SELECT ` + accountColumns + ` FROM accounts`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY bank_code, account_number`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, nil
}

func (r *AccountRepo) UpsertBatch(ctx context.Context, credentialID uuid.UUID, accounts []Account) error {
	if len(accounts) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin upsert batch: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO accounts (bank_code, account_number, currency, account_type, credential_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bank_code, account_number) DO UPDATE SET
			currency = EXCLUDED.currency,
			account_type = EXCLUDED.account_type,
			credential_id = EXCLUDED.credential_id,
			updated_at = now()`

	for i, a := range accounts {
		_, err := tx.Exec(ctx, query,
			a.BankCode, a.AccountNumber, a.Currency, a.AccountType, credentialID,
		)
		if err != nil {
			return fmt.Errorf("upsert account %d (%s/%s): %w", i, a.BankCode, a.AccountNumber, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit upsert batch: %w", err)
	}
	return nil
}

func (r *AccountRepo) UpdateLastSynced(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE accounts SET last_synced_at = now(), updated_at = now() WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("update last synced: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("account %s: %w", id, ErrNotFound)
	}
	return nil
}
