// Package bank defines the common structs and logic used throughout bank
// implementations.
package bank

import "context"

// Scraper defines the interface for bank-specific scraping operations.
type Scraper interface {
	// Login authenticates with the bank and establishes a session
	Login(ctx context.Context, session *Session, accountID string) (*Session, error)
}

// Code identifies a supported bank.
type Code string

// Supported bank codes.
const (
	BankBBVA      Code = "BBVA"
	BankInterbank Code = "INTERBANK"
	BankBCP       Code = "BCP"
)
