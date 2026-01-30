// Package bank defines the common structs and logic used throughout bank
// implementations.
package bank

import "context"

type BankScraper interface {
	// Login authenticates with the bank and establishes a session
	Login(ctx context.Context, session *Session, accountID string) (*Session, error)
}

type BankCode string

const (
	BankBBVA      BankCode = "BBVA"
	BankInterbank BankCode = "INTERBANK"
	BankBCP       BankCode = "BCP"
)
