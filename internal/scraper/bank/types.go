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

// Transaction represents a transaction for a bank account.
// it uses int64 for the amount and assumes a 2 point precision.
type Transaction struct {
	// Identity
	ID        string // Bank's document/reference number (e.g., BBVA's N. Doc)
	Reference string // 	Secondary reference if available (e.g., transfer reference)

	// Dates
	Date      time.Time // F. Operacion
	ValueDate time.Time // F. Valor

	// Transaction details
	Description string
	Amount      int64           // Always positive, in cents (2 decimal places)
	Type        TransactionType // CREDIT (money in) or DEBIT (money out)

	// Balance (optional - only populated if bank provides it)
	BalanceAfter int64

	// Bank-specific metadata (optional)
	BankCode string            // Transaction type code (e.g., BBVA's Código: "015", "527")
	Extra    map[string]string // Extra metadata. e.g., Store "Codigo": "015", "Office": "0437"
}

type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyPEN Currency = "PEN"
)

type TransactionType string

const (
	TransactionCredit TransactionType = "CREDIT"
	TransactionDebit  TransactionType = "DEBIT"
)
