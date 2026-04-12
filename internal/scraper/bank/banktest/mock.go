// Package banktest provides test doubles for the bank.Scraper interface.
package banktest

import (
	"context"

	"github.com/aynifx/bank-scraper/internal/scraper/bank"
)

// MockScraper is a configurable test double for bank.Scraper.
// Set fields before use to control return values; check counters after use to verify calls.
type MockScraper struct {
	// Return values
	LoginSession    *bank.Session
	LoginErr        error
	Balances        []bank.Balance
	BalanceErr      error
	Transactions    []bank.Transaction
	TransactionsErr error

	// Call counters
	LoginCalled  int
	LogoutCalled int
	CloseCalled  int
}

// Login implements bank.Scraper.
func (m *MockScraper) Login(_ context.Context, _ map[string]string) (*bank.Session, error) {
	m.LoginCalled++
	if m.LoginErr != nil {
		return nil, m.LoginErr
	}
	return m.LoginSession, nil
}

// GetBalance implements bank.Scraper.
func (m *MockScraper) GetBalance(_ context.Context) ([]bank.Balance, error) {
	if m.BalanceErr != nil {
		return nil, m.BalanceErr
	}
	return m.Balances, nil
}

// GetTransactions implements bank.Scraper.
func (m *MockScraper) GetTransactions(_ context.Context, _ string, _ int) ([]bank.Transaction, error) {
	if m.TransactionsErr != nil {
		return nil, m.TransactionsErr
	}
	return m.Transactions, nil
}

// Logout implements bank.Scraper.
func (m *MockScraper) Logout(_ context.Context) error {
	m.LogoutCalled++
	return nil
}

// Close implements bank.Scraper.
func (m *MockScraper) Close() error {
	m.CloseCalled++
	return nil
}
