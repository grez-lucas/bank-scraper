package store

import (
	"embed"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // register pgx/v5 database driver for golang-migrate
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending migrations.
func RunMigrations(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer closeMigrate(m)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// RollbackMigration rolls back the last migration.
func RollbackMigration(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer closeMigrate(m)

	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("rollback migration: %w", err)
	}
	return nil
}

// MigrationVersion returns the current migration version and dirty state.
func MigrationVersion(databaseURL string) (uint, bool, error) {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return 0, false, err
	}
	defer closeMigrate(m)

	return m.Version()
}

func closeMigrate(m *migrate.Migrate) {
	srcErr, dbErr := m.Close()
	_ = srcErr
	_ = dbErr
}

func newMigrate(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	// golang-migrate's pgx/v5 driver expects the "pgx5://" scheme
	connStr, err := pgxMigrateURL(databaseURL)
	if err != nil {
		return nil, err
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, connStr)
	if err != nil {
		return nil, fmt.Errorf("create migrate instance: %w", err)
	}
	return m, nil
}

// pgxMigrateURL converts a standard postgres:// or postgresql:// URL to
// the pgx5:// scheme expected by golang-migrate's pgx/v5 driver.
func pgxMigrateURL(databaseURL string) (string, error) {
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(databaseURL, prefix) {
			return "pgx5://" + strings.TrimPrefix(databaseURL, prefix), nil
		}
	}
	return "", fmt.Errorf("unsupported database URL scheme (expected postgres://)")
}
