package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a connection pool to the test database.
// Skips the test if DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping: requires DATABASE_URL env var (run `make db-up` first)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)

	// Run migrations to ensure schema is up to date
	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return pool
}

// truncateTables clears all data from tables in reverse FK order.
func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Order matters: reverse FK dependency
	tables := []string{"audit_logs", "bank_credentials", "sessions", "users"}
	for _, table := range tables {
		if _, err := pool.Exec(ctx, "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// createTestUser inserts a minimal user for tests that need a FK reference.
func createTestUser(t *testing.T, repo *UserRepo) *User {
	t.Helper()

	u := &User{
		Username:      "testuser",
		PasswordHash:  "$2a$12$fakehashfortesting000000000000000000000000000000",
		TOTPSecretEnc: []byte("encrypted-totp-secret"),
		TOTPSecretDEK: []byte("encrypted-dek"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u
}
