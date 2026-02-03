package bank

import "time"

type Session struct {
	ID        string
	BankCode  BankCode
	ExpiresAt time.Time
	// Internal: browser page reference (not exported)
	page any
}

// Balance represents the balance of an account for a certain currency
// it uses int64 and assumes a 2 point precision.
type Balance struct {
	AccountID        string
	Currency         Currency
	AvailableBalance int64
	CurrentBalance   int64
	FetchedAt        time.Time
}

type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyPEN Currency = "PEN"
)
