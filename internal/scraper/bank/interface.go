// Package bank defines the common structs and logic used throughout bank
// implementations.
package bank

import "context"

// Scraper defines the interface for bank-specific scraping operations.
// Each bank implementation (BBVA, Interbank, BCP) must satisfy this interface.
type Scraper interface {
	// Login authenticates with the bank using a generic credential field map.
	// The map keys are bank-specific (e.g., "company_code", "user_code", "password" for BBVA).
	// Returns an authenticated session or an error.
	Login(ctx context.Context, credentials map[string]string) (*Session, error)

	// GetBalance fetches account balances for all accounts visible to the authenticated session.
	// Must be called after a successful Login.
	GetBalance(ctx context.Context) ([]Balance, error)

	// GetTransactions fetches transaction history for a specific account.
	// count is the desired number of transactions; the scraper may clamp to bank-specific limits.
	// Must be called after a successful Login.
	GetTransactions(ctx context.Context, accountID string, count int) ([]Transaction, error)

	// Logout performs a clean logout from the bank portal, clearing the session.
	Logout(ctx context.Context) error

	// Close shuts down the browser and releases all resources.
	Close() error
}

// Code identifies a supported bank.
type Code string

// Supported bank codes.
const (
	BankBBVA      Code = "BBVA"
	BankInterbank Code = "INTERBANK"
	BankBCP       Code = "BCP"
)

// ScraperFactory creates a new Scraper instance for the given bank code.
type ScraperFactory func(bankCode Code) (Scraper, error)
