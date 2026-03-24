// Package service contains business logic for the API gateway.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// DiscoveryService discovers bank accounts by logging in and fetching balances.
// It uses a dedicated scraper instance (not the API's live singleton) to avoid
// interfering with active sessions.
type DiscoveryService struct {
	accounts store.AccountRepository
	factory  bank.ScraperFactory
	logger   *slog.Logger
}

// NewDiscoveryService creates a new DiscoveryService.
func NewDiscoveryService(accounts store.AccountRepository, factory bank.ScraperFactory, logger *slog.Logger) *DiscoveryService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DiscoveryService{
		accounts: accounts,
		factory:  factory,
		logger:   logger,
	}
}

// Discover logs into a bank, fetches all visible accounts via GetBalance,
// and upserts them into the accounts table. Returns the discovered accounts.
//
// It manages its own scraper lifecycle: create → login → GetBalance → logout → close.
func (d *DiscoveryService) Discover(ctx context.Context, bankCode string, creds map[string]string, credentialID uuid.UUID) ([]store.Account, error) {
	code := bank.Code(bankCode)

	scraper, err := d.factory(code)
	if err != nil {
		return nil, fmt.Errorf("create scraper for %s: %w", bankCode, err)
	}

	// Ensure cleanup regardless of outcome.
	// Use context.Background for cleanup since the original ctx may be cancelled.
	loggedIn := false
	defer func() {
		if loggedIn {
			_ = scraper.Logout(context.Background())
		}
		_ = scraper.Close()
	}()

	if _, err := scraper.Login(ctx, creds); err != nil {
		return nil, fmt.Errorf("login to %s: %w", bankCode, err)
	}
	loggedIn = true

	balances, err := scraper.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("get balance for %s: %w", bankCode, err)
	}

	if len(balances) == 0 {
		d.logger.Info("no accounts discovered", slog.String("bank", bankCode))
		return nil, nil
	}

	accounts := balancesToAccounts(bankCode, balances)

	if err := d.accounts.UpsertBatch(ctx, credentialID, accounts); err != nil {
		return nil, fmt.Errorf("upsert accounts for %s: %w", bankCode, err)
	}

	d.logger.Info("accounts discovered",
		slog.String("bank", bankCode),
		slog.Int("count", len(accounts)))

	return accounts, nil
}

// balancesToAccounts maps scraper Balance results to store Account structs.
func balancesToAccounts(bankCode string, balances []bank.Balance) []store.Account {
	accounts := make([]store.Account, len(balances))
	for i, b := range balances {
		accounts[i] = store.Account{
			BankCode:      bankCode,
			AccountNumber: b.AccountID,
			Currency:      string(b.Currency),
			AccountType:   store.AccountTypeChecking,
		}
	}
	return accounts
}
