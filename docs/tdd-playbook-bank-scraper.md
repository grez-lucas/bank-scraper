# TDD Playbook

Author: Lucas Grez
Created time: January 29, 2026 10:43 AM
Last edited by: Lucas Grez
Last updated time: January 29, 2026 2:50 PM

# TDD Playbook: Bank Scraper Development with Go + Rod

**Document Version:** 1.0

**Created:** January 2026

**Purpose:** Step-by-step guide for test-driven scraper development

---

## Core Principle: Separate What You Can Test from What You Can't

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     SCRAPER ARCHITECTURE                                │
│                                                                         │
│   ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    │
│   │   Navigation    │───▶│   Extraction    │───▶│    Parsing      │    │
│   │   (Browser)     │    │   (HTML/DOM)    │    │    (Data)       │    │
│   └─────────────────┘    └─────────────────┘    └─────────────────┘    │
│                                                                         │
│   Hard to test           Medium                  Easy to test          │
│   (use recordings)       (use fixtures)          (pure functions)      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

```

**Key insight:** Most scraper bugs are in parsing logic, not browser automation. Structure your code so parsing is isolated and easily testable.

---

## 1. Project Structure for TDD

```
internal/scraper/bank/
├── interface.go           # Common contract (BankScraper interface)
├── types.go               # Shared types (Balance, Transaction, etc.)
├── errors.go              # Custom error types
│
├── bbva/
│   ├── scraper.go         # BBVAScraper struct + browser logic
│   ├── scraper_test.go    # Integration tests (with recordings)
│   ├── parser.go          # Pure parsing functions
│   ├── parser_test.go     # Unit tests (fast, no browser)
│   ├── selectors.go       # CSS selectors as constants
│   └── testdata/
│       ├── fixtures/      # Static HTML snapshots
│       │   ├── login_page.html
│       │   ├── dashboard.html
│       │   ├── balance_pen.html
│       │   ├── balance_usd.html
│       │   └── transactions.html
│       └── recordings/    # Rod traces for replay
│           └── login_flow.trace/
│
├── interbank/
│   └── ... (same structure)
│
└── bcp/
    └── ... (same structure)

```

---

## 2. The Interface Contract

### interface.go

```go
package bank

import (
	"context"
	"time"
)

// BankScraper defines the contract all bank implementations must follow
type BankScraper interface {
	// Login authenticates with the bank and establishes a session
	Login(ctx context.Context, credentials Credentials) (*Session, error)

	// GetBalance retrieves the current balance for an account
	GetBalance(ctx context.Context, session *Session, accountID string) (*Balance, error)

	// GetTransactions retrieves transactions within a date range
	GetTransactions(ctx context.Context, session *Session, accountID string, from, to time.Time) ([]Transaction, error)

	// Logout terminates the session
	Logout(ctx context.Context, session *Session) error

	// Close releases all browser resources
	Close() error
}

// BankCode identifies the bank
type BankCode string

const (
	BankBBVA      BankCode = "BBVA"
	BankInterbank BankCode = "INTERBANK"
	BankBCP       BankCode = "BCP"
)

```

### types.go

```go
package bank

import "time"

type Credentials struct {
	Username string
	Password string
}

type Session struct {
	ID        string
	BankCode  BankCode
	ExpiresAt time.Time
	// Internal: browser page reference (not exported)
	page interface{}
}

type Balance struct {
	AccountID        string
	Currency         Currency
	AvailableBalance float64
	CurrentBalance   float64
	FetchedAt        time.Time
}

type Transaction struct {
	ID           string
	Date         time.Time
	Description  string
	Amount       float64
	Type         TransactionType
	BalanceAfter float64
	Reference    string
}

type Currency string

const (
	CurrencyPEN Currency = "PEN"
	CurrencyUSD Currency = "USD"
)

type TransactionType string

const (
	TransactionCredit TransactionType = "CREDIT"
	TransactionDebit  TransactionType = "DEBIT"
)

```

### errors.go

```go
package bank

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")
	ErrAccountNotFound    = errors.New("account not found")
	ErrBankUnavailable    = errors.New("bank website unavailable")
	ErrParsingFailed      = errors.New("failed to parse bank response")
	ErrTimeout            = errors.New("operation timed out")
)

// ScraperError provides detailed error context
type ScraperError struct {
	BankCode  BankCode
	Operation string
	Cause     error
	Details   string
}

func (e *ScraperError) Error() string {
	return fmt.Sprintf("[%s] %s failed: %v - %s", e.BankCode, e.Operation, e.Cause, e.Details)
}

func (e *ScraperError) Unwrap() error {
	return e.Cause
}

```

---

## 3. The TDD Cycle for Scrapers

### Phase 1: Capture Real HTML (One-time, Manual)

Before writing any code, capture real HTML from the bank:

```bash
# Record a session and save HTML snapshots
./scripts/capture-fixtures.sh bbva

```

**capture-fixtures.sh:**

```bash
#!/bin/bash
BANK=$1
mkdir -p internal/scraper/bank/$BANK/testdata/fixtures

echo "🎬 Opening browser for $BANK - save pages manually"
echo "   Save each page as HTML to: internal/scraper/bank/$BANK/testdata/fixtures/"
echo "   Required pages:"
echo "     - login_page.html"
echo "     - dashboard.html"
echo "     - balance_pen.html"
echo "     - balance_usd.html"
echo "     - transactions.html"
echo "     - login_error.html (invalid credentials)"

# Open browser for manual capture
go run ./scripts/capture/main.go --bank=$BANK

```

### Phase 2: Write Parser Tests FIRST (TDD)

**Always start with parsers. This is where TDD shines.**

```
┌─────────────────────────────────────────────────────────┐
│              TDD CYCLE FOR PARSERS                      │
│                                                         │
│    ┌─────────┐    ┌─────────┐    ┌─────────┐           │
│    │  RED    │───▶│  GREEN  │───▶│ REFACTOR│───┐       │
│    │         │    │         │    │         │   │       │
│    │ Write   │    │ Make it │    │ Clean   │   │       │
│    │ failing │    │ pass    │    │ up      │   │       │
│    │ test    │    │         │    │         │   │       │
│    └─────────┘    └─────────┘    └─────────┘   │       │
│         ▲                                      │       │
│         └──────────────────────────────────────┘       │
│                                                         │
│    Tests run in < 1 second (no browser!)               │
└─────────────────────────────────────────────────────────┘

```

---

## 4. Example: Building the BBVA Scraper with TDD

### Step 1: Define Selectors (Constants)

**bbva/selectors.go:**

```go
package bbva

// CSS selectors for BBVA web portal
// Keep these separate so they're easy to update when the site changes
const (
	// Login page
	SelectorUsernameInput = "#username"
	SelectorPasswordInput = "#password"
	SelectorLoginButton   = "#btn-login"
	SelectorLoginError    = ".error-message"

	// Dashboard
	SelectorAccountList     = ".account-list .account-item"
	SelectorAccountNumber   = ".account-number"
	SelectorAccountCurrency = ".account-currency"

	// Balance page
	SelectorAvailableBalance = ".available-balance .amount"
	SelectorCurrentBalance   = ".current-balance .amount"
	SelectorCurrencySymbol   = ".currency-symbol"

	// Transactions
	SelectorTransactionTable = "#transactions-table"
	SelectorTransactionRow   = "tbody tr"
	SelectorTxDate           = ".tx-date"
	SelectorTxDescription    = ".tx-description"
	SelectorTxAmount         = ".tx-amount"
	SelectorTxBalance        = ".tx-balance"
)

```

### Step 2: Write Parser Test FIRST (Red)

**bbva/parser_test.go:**

```go
package bbva

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourcompany/bank-scraper/internal/scraper/bank"
)

// loadFixture reads HTML from testdata/fixtures
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/fixtures/" + name)
	require.NoError(t, err, "failed to load fixture: %s", name)
	return string(data)
}

func TestParseBalance_PEN(t *testing.T) {
	// Arrange
	html := loadFixture(t, "balance_pen.html")

	// Act
	balance, err := ParseBalance(html)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, bank.CurrencyPEN, balance.Currency)
	assert.Equal(t, 15234.56, balance.AvailableBalance)
	assert.Equal(t, 15234.56, balance.CurrentBalance)
}

func TestParseBalance_USD(t *testing.T) {
	html := loadFixture(t, "balance_usd.html")

	balance, err := ParseBalance(html)

	require.NoError(t, err)
	assert.Equal(t, bank.CurrencyUSD, balance.Currency)
	assert.Equal(t, 5000.00, balance.AvailableBalance)
}

func TestParseBalance_InvalidHTML(t *testing.T) {
	html := "<html><body>Something unexpected</body></html>"

	_, err := ParseBalance(html)

	assert.ErrorIs(t, err, bank.ErrParsingFailed)
}

func TestParseTransactions(t *testing.T) {
	html := loadFixture(t, "transactions.html")

	transactions, err := ParseTransactions(html)

	require.NoError(t, err)
	require.Len(t, transactions, 5) // Expect 5 transactions in fixture

	// Verify first transaction
	tx := transactions[0]
	assert.Equal(t, "2024-12-15", tx.Date.Format("2006-01-02"))
	assert.Equal(t, "TRANSFERENCIA DE TERCEROS", tx.Description)
	assert.Equal(t, 1500.00, tx.Amount)
	assert.Equal(t, bank.TransactionCredit, tx.Type)
}

func TestParseTransactions_Empty(t *testing.T) {
	html := loadFixture(t, "transactions_empty.html")

	transactions, err := ParseTransactions(html)

	require.NoError(t, err)
	assert.Empty(t, transactions)
}

func TestParseLoginError(t *testing.T) {
	html := loadFixture(t, "login_error.html")

	errMsg, hasError := ParseLoginError(html)

	assert.True(t, hasError)
	assert.Contains(t, errMsg, "credenciales")
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
		wantErr  bool
	}{
		{"simple", "1,234.56", 1234.56, false},
		{"no decimals", "1,234", 1234.00, false},
		{"negative", "-500.00", -500.00, false},
		{"with currency symbol", "S/ 1,234.56", 1234.56, false},
		{"USD symbol", "$ 5,000.00", 5000.00, false},
		{"empty", "", 0, true},
		{"invalid", "abc", 0, true},
		{"peruvian format", "1.234,56", 1234.56, false}, // Thousand sep = dot
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAmount(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestParseDatePeru(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{"standard", "15/12/2024", time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC), false},
		{"with time", "15/12/2024 14:30", time.Date(2024, 12, 15, 14, 30, 0, 0, time.UTC), false},
		{"invalid", "2024-12-15", time.Time{}, true}, // Wrong format
		{"empty", "", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDatePeru(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

```

**Run the test (it fails - RED):**

```bash
go test ./internal/scraper/bank/bbva/... -v
# FAIL: ParseBalance undefined

```

### Step 3: Implement Parser (Green)

**bbva/parser.go:**

```go
package bbva

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/yourcompany/bank-scraper/internal/scraper/bank"
)

// ParseBalance extracts balance information from the balance page HTML
func ParseBalance(html string) (*bank.Balance, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse HTML: %v", bank.ErrParsingFailed, err)
	}

	// Extract available balance
	availableText := doc.Find(SelectorAvailableBalance).Text()
	if availableText == "" {
		return nil, fmt.Errorf("%w: available balance not found", bank.ErrParsingFailed)
	}

	available, err := ParseAmount(availableText)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid available balance: %v", bank.ErrParsingFailed, err)
	}

	// Extract current balance
	currentText := doc.Find(SelectorCurrentBalance).Text()
	current, err := ParseAmount(currentText)
	if err != nil {
		// Current balance might equal available if not shown separately
		current = available
	}

	// Detect currency
	currency := detectCurrency(doc)

	return &bank.Balance{
		Currency:         currency,
		AvailableBalance: available,
		CurrentBalance:   current,
		FetchedAt:        time.Now(),
	}, nil
}

// ParseTransactions extracts transaction list from transactions page HTML
func ParseTransactions(html string) ([]bank.Transaction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse HTML: %v", bank.ErrParsingFailed, err)
	}

	var transactions []bank.Transaction

	doc.Find(SelectorTransactionRow).Each(func(i int, row *goquery.Selection) {
		tx, err := parseTransactionRow(row)
		if err != nil {
			// Log but continue - don't fail on single bad row
			return
		}
		transactions = append(transactions, *tx)
	})

	return transactions, nil
}

func parseTransactionRow(row *goquery.Selection) (*bank.Transaction, error) {
	dateText := strings.TrimSpace(row.Find(SelectorTxDate).Text())
	date, err := ParseDatePeru(dateText)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(row.Find(SelectorTxDescription).Text())

	amountText := strings.TrimSpace(row.Find(SelectorTxAmount).Text())
	amount, err := ParseAmount(amountText)
	if err != nil {
		return nil, err
	}

	balanceText := strings.TrimSpace(row.Find(SelectorTxBalance).Text())
	balance, _ := ParseAmount(balanceText) // Balance might be optional

	txType := bank.TransactionCredit
	if amount < 0 {
		txType = bank.TransactionDebit
		amount = -amount // Store as positive
	}

	return &bank.Transaction{
		ID:           generateTxID(date, description, amount),
		Date:         date,
		Description:  description,
		Amount:       amount,
		Type:         txType,
		BalanceAfter: balance,
	}, nil
}

// ParseLoginError checks if the page contains a login error
func ParseLoginError(html string) (string, bool) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	errMsg := doc.Find(SelectorLoginError).Text()
	return strings.TrimSpace(errMsg), errMsg != ""
}

// ParseAmount converts a formatted amount string to float64
// Handles: "1,234.56", "S/ 1,234.56", "$ 5,000.00", "1.234,56" (EU format)
func ParseAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}

	// Remove currency symbols and spaces
	s = regexp.MustCompile(`[S/$\s]`).ReplaceAllString(s, "")

	// Detect format: if comma is after dot, it's decimal separator (EU format)
	dotIdx := strings.LastIndex(s, ".")
	commaIdx := strings.LastIndex(s, ",")

	if commaIdx > dotIdx {
		// European format: 1.234,56 -> 1234.56
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	} else {
		// US format: 1,234.56 -> 1234.56
		s = strings.ReplaceAll(s, ",", "")
	}

	return strconv.ParseFloat(s, 64)
}

// ParseDatePeru parses dates in Peruvian format (DD/MM/YYYY)
func ParseDatePeru(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}

	// Try with time first
	if t, err := time.Parse("02/01/2006 15:04", s); err == nil {
		return t, nil
	}

	// Try date only
	if t, err := time.Parse("02/01/2006", s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("invalid date format: %s", s)
}

func detectCurrency(doc *goquery.Document) bank.Currency {
	symbol := doc.Find(SelectorCurrencySymbol).Text()
	if strings.Contains(symbol, "$") || strings.Contains(symbol, "USD") {
		return bank.CurrencyUSD
	}
	return bank.CurrencyPEN
}

func generateTxID(date time.Time, desc string, amount float64) string {
	// Create deterministic ID from transaction data
	data := fmt.Sprintf("%s-%s-%.2f", date.Format("20060102"), desc, amount)
	// Simple hash - in production use proper hashing
	return fmt.Sprintf("%x", data)[:16]
}

```

**Run tests again (GREEN):**

```bash
go test ./internal/scraper/bank/bbva/... -v
# PASS

```

### Step 4: Refactor if Needed

Look for:

- Duplicated code → Extract functions
- Complex conditionals → Simplify
- Magic strings → Constants
- Missing error context → Wrap errors

---

## 5. Writing the Scraper (Integration with Browser)

Now that parsers are tested, the scraper becomes thin glue code.

**bbva/scraper.go:**

```go
package bbva

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/yourcompany/bank-scraper/internal/scraper/bank"
)

const (
	baseURL      = "https://www.bbva.pe"
	loginURL     = baseURL + "/personas/login"
	dashboardURL = baseURL + "/personas/dashboard"

	defaultTimeout = 30 * time.Second
)

type BBVAScraper struct {
	browser *rod.Browser
	timeout time.Duration
}

// Option pattern for configuration
type Option func(*BBVAScraper)

func WithTimeout(d time.Duration) Option {
	return func(s *BBVAScraper) {
		s.timeout = d
	}
}

func NewBBVAScraper(opts ...Option) (*BBVAScraper, error) {
	// Launch browser with stealth mode
	url, err := launcher.New().
		Headless(true).
		Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	s := &BBVAScraper{
		browser: browser,
		timeout: defaultTimeout,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *BBVAScraper) Login(ctx context.Context, creds bank.Credentials) (*bank.Session, error) {
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: loginURL})
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   err.Error(),
		}
	}

	// Set timeout for this operation
	page = page.Timeout(s.timeout)

	// Wait for login form
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("login page load failed: %w", err)
	}

	// Fill credentials
	if err := page.MustElement(SelectorUsernameInput).Input(creds.Username); err != nil {
		return nil, fmt.Errorf("failed to enter username: %w", err)
	}

	if err := page.MustElement(SelectorPasswordInput).Input(creds.Password); err != nil {
		return nil, fmt.Errorf("failed to enter password: %w", err)
	}

	// Click login
	if err := page.MustElement(SelectorLoginButton).Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("failed to click login: %w", err)
	}

	// Wait for navigation
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("post-login load failed: %w", err)
	}

	// Check for login error
	html, _ := page.HTML()
	if errMsg, hasError := ParseLoginError(html); hasError {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrInvalidCredentials,
			Details:   errMsg,
		}
	}

	return &bank.Session{
		ID:        generateSessionID(),
		BankCode:  bank.BankBBVA,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		page:      page,
	}, nil
}

func (s *BBVAScraper) GetBalance(ctx context.Context, session *bank.Session, accountID string) (*bank.Balance, error) {
	page := session.page.(*rod.Page)
	page = page.Timeout(s.timeout)

	// Navigate to account balance page
	balanceURL := fmt.Sprintf("%s/cuentas/%s/saldo", baseURL, accountID)
	if err := page.Navigate(balanceURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to balance: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("balance page load failed: %w", err)
	}

	// Get HTML and parse (parsing is already tested!)
	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("failed to get page HTML: %w", err)
	}

	balance, err := ParseBalance(html)
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     err,
			Details:   "HTML structure may have changed",
		}
	}

	balance.AccountID = accountID
	return balance, nil
}

func (s *BBVAScraper) GetTransactions(ctx context.Context, session *bank.Session, accountID string, from, to time.Time) ([]bank.Transaction, error) {
	page := session.page.(*rod.Page)
	page = page.Timeout(s.timeout)

	// Navigate to transactions page with date filter
	txURL := fmt.Sprintf("%s/cuentas/%s/movimientos?desde=%s&hasta=%s",
		baseURL, accountID,
		from.Format("02-01-2006"),
		to.Format("02-01-2006"),
	)

	if err := page.Navigate(txURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to transactions: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("transactions page load failed: %w", err)
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("failed to get page HTML: %w", err)
	}

	// Parsing is already tested!
	return ParseTransactions(html)
}

func (s *BBVAScraper) Logout(ctx context.Context, session *bank.Session) error {
	if session == nil || session.page == nil {
		return nil
	}

	page := session.page.(*rod.Page)
	return page.Close()
}

func (s *BBVAScraper) Close() error {
	if s.browser != nil {
		return s.browser.Close()
	}
	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("bbva-%d", time.Now().UnixNano())
}

```

---

## 6. Integration Tests (With Rod Recordings)

For browser integration tests, use Rod's recording/replay feature.

**bbva/scraper_test.go:**

```go
package bbva

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourcompany/bank-scraper/internal/scraper/bank"
)

// TestMode controls how tests interact with the bank
type TestMode string

const (
	TestModeMock   TestMode = "mock"   // Use static fixtures (fastest)
	TestModeReplay TestMode = "replay" // Replay recorded sessions
	TestModeLive   TestMode = "live"   // Hit real bank (dangerous!)
)

func getTestMode() TestMode {
	mode := os.Getenv("SCRAPER_TEST_MODE")
	if mode == "" {
		return TestModeMock
	}
	return TestMode(mode)
}

// skipUnlessMode skips test if not in specified mode
func skipUnlessMode(t *testing.T, required TestMode) {
	if getTestMode() != required {
		t.Skipf("Skipping: requires SCRAPER_TEST_MODE=%s", required)
	}
}

// Integration test - runs only in replay/live mode
func TestBBVAScraper_Login_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	scraper, err := NewBBVAScraper(
		WithTimeout(30 * time.Second),
	)
	require.NoError(t, err)
	defer scraper.Close()

	// Load test credentials (from env or config)
	creds := bank.Credentials{
		Username: os.Getenv("BBVA_TEST_USER"),
		Password: os.Getenv("BBVA_TEST_PASS"),
	}

	ctx := context.Background()
	session, err := scraper.Login(ctx, creds)

	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.Equal(t, bank.BankBBVA, session.BankCode)
}

func TestBBVAScraper_Login_InvalidCredentials(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	scraper, err := NewBBVAScraper()
	require.NoError(t, err)
	defer scraper.Close()

	creds := bank.Credentials{
		Username: "invalid_user",
		Password: "invalid_pass",
	}

	ctx := context.Background()
	_, err = scraper.Login(ctx, creds)

	require.Error(t, err)
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr)
	assert.ErrorIs(t, scraperErr.Cause, bank.ErrInvalidCredentials)
}

// Full flow test - runs only in live mode (rare)
func TestBBVAScraper_FullFlow_Live(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	scraper, err := NewBBVAScraper()
	require.NoError(t, err)
	defer scraper.Close()

	ctx := context.Background()

	// 1. Login
	creds := bank.Credentials{
		Username: os.Getenv("BBVA_TEST_USER"),
		Password: os.Getenv("BBVA_TEST_PASS"),
	}
	session, err := scraper.Login(ctx, creds)
	require.NoError(t, err)

	// 2. Get Balance
	accountID := os.Getenv("BBVA_TEST_ACCOUNT")
	balance, err := scraper.GetBalance(ctx, session, accountID)
	require.NoError(t, err)
	assert.Greater(t, balance.AvailableBalance, 0.0)

	// 3. Get Transactions
	to := time.Now()
	from := to.AddDate(0, 0, -7)
	txs, err := scraper.GetTransactions(ctx, session, accountID, from, to)
	require.NoError(t, err)
	t.Logf("Found %d transactions", len(txs))

	// 4. Logout
	err = scraper.Logout(ctx, session)
	require.NoError(t, err)
}

```

---

## 7. Recording Sessions for Replay (Manual HAR Recording)

Banks like BBVA use advanced bot detection (Akamai Bot Manager) that blocks automated browsers even with stealth mode. The solution is to record HTTP traffic manually using Chrome DevTools.

### Why Manual Recording?

During development of the BBVA scraper, we discovered that Akamai Bot Manager detects automated browsers through:
- TLS fingerprinting
- JavaScript execution patterns
- Mouse/keyboard behavior timing
- IP reputation and request patterns

Even passive CDP network monitoring triggered 403 errors with "Algo salió mal" (Something went wrong) messages. The solution is to record HTTP traffic manually using your regular Chrome browser.

### Recording Steps

1. **Open Chrome** (your regular browser, not automated)

2. **Open DevTools**: Press `F12` or `Cmd+Option+I` (Mac) / `Ctrl+Shift+I` (Windows/Linux)

3. **Go to Network tab**

4. **Enable "Preserve log"**: Check the checkbox to keep requests across page navigations

5. **Clear existing entries**: Click the 🚫 icon to start fresh

6. **Navigate to the bank login page**:
   - BBVA: `https://www.bbvanetcash.pe/DFAUTH85/mult/KDPOSolicitarCredenciales_es.html`

7. **Complete the login flow** with valid credentials

8. **Wait for the dashboard/success page** to fully load

9. **Export HAR file**: Right-click in the Network panel → **"Save all as HAR with content"**

10. **Save to recordings directory**:
    ```
    internal/scraper/bank/{bank}/testdata/recordings/{scenario}.har.json
    ```

### HAR File Naming Convention

| Scenario | Filename |
|----------|----------|
| Successful login | `login_success.har.json` |
| Invalid credentials | `login_error.har.json` |
| Session expired | `session_expired.har.json` |
| Balance fetch | `balance_success.har.json` |
| Transactions list | `transactions_success.har.json` |

### Sanitize Before Commit

**CRITICAL:** HAR files contain sensitive data including passwords, session tokens, and cookies. Always sanitize before committing.

```bash
# Sanitize using conventional path
go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login_success

# Or specify paths directly
go run ./scripts/sanitize-har/main.go -input=recording.har.json -output=sanitized.har.json

# Preview what will be redacted (dry run)
go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login_success -dry-run
```

The sanitizer automatically redacts:
- Passwords and credentials (`password`, `clave`, `secret`)
- Session tokens and cookies (`token`, `session`, `auth`, `jwt`)
- API keys (`api_key`, `apikey`)
- Sensitive headers (`Authorization`, `Cookie`, `Set-Cookie`, `X-CSRF-Token`)

### Chrome vs Simplified HAR Format

Chrome DevTools exports HAR 1.2 format with a `log` wrapper:
```json
{
  "log": {
    "version": "1.2",
    "entries": [...]
  }
}
```

Our `LoadHAR()` function auto-detects this format and converts it to our simplified internal format. Both formats work transparently with the replay system.

### Running Replay Tests

```bash
# Run replay tests for BBVA
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v

# Run specific replay test
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v -run TestBBVAScraper_Login_ReplaySuccess
```

### Troubleshooting

**Recording shows 0 requests:**
- Ensure "Preserve log" is checked before navigating
- Make sure you waited for pages to fully load

**Test fails with "no recording found":**
- Check the HAR file path matches what the test expects
- Verify the HAR file isn't empty
- Run with verbose mode: `WithVerbose(true)` in test setup

**403 errors in recording:**
- This indicates bot detection triggered during your session
- Try again from a different browser profile
- Wait a few minutes and retry (may be rate limiting)
- Use a different network/IP if available

---

## 8. Makefile for Test Workflow

```makefile
.PHONY: test test-unit test-integration test-live record-bbva

# Run only unit tests (fast, no browser)
test-unit:
	go test ./internal/scraper/bank/*/parser*.go -v

# Run with recorded sessions
test-integration:
	SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/... -v

# Run against live banks (dangerous!)
test-live:
	@echo "⚠️  WARNING: This will hit live bank websites!"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ]
	SCRAPER_TEST_MODE=live go test ./internal/scraper/bank/... -v -count=1

# Default: only unit tests
test: test-unit

# Sanitize HAR recordings before commit
sanitize-har:
	@echo "Sanitizing HAR files..."
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login_success

# Update fixtures (save current HTML)
update-fixtures:
	@echo "Open browser, navigate to each page, save as HTML to testdata/fixtures/"
	@echo "Required files: login_page.html, dashboard.html, balance_pen.html, etc."

```

---

## 9. TDD Cheat Sheet

### Daily Workflow

```
┌─────────────────────────────────────────────────────────────────────┐
│                     DAILY TDD WORKFLOW                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. Write parser test (RED)                    ← 2 minutes          │
│     └─▶ go test ./internal/.../bbva/parser_test.go -run TestNew     │
│                                                                     │
│  2. Implement parser function (GREEN)          ← 5-15 minutes       │
│     └─▶ go test ./internal/.../bbva/parser_test.go -run TestNew     │
│                                                                     │
│  3. Refactor if needed                         ← 2-5 minutes        │
│     └─▶ go test ./internal/.../bbva/... (all parser tests)          │
│                                                                     │
│  4. Update scraper to use new parser           ← 5 minutes          │
│     └─▶ Thin glue code, already tested                              │
│                                                                     │
│  5. Integration test (replay mode)             ← As needed          │
│     └─▶ SCRAPER_TEST_MODE=replay go test ...                        │
│                                                                     │
│  6. Live test (weekly or on demand)            ← Rare               │
│     └─▶ SCRAPER_TEST_MODE=live go test ...                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

```

### When Something Breaks in Production

```
1. Capture new HTML from bank (manually or via recording)
2. Add as new fixture: balance_pen_v2.html
3. Write failing test that uses new fixture
4. Fix parser to handle both old and new format
5. All tests pass → Deploy

```

### Test File Naming

| File | Contains | Runs When |
| --- | --- | --- |
| `parser_test.go` | Pure function tests | Always (fast) |
| `scraper_test.go` | Browser tests | Integration/Live mode |
| `scraper_mock_test.go` | Mock-based tests | Always |

---

## 10. Summary: The Golden Rules

1. **Parsers are pure functions** → Easy to test, no browser needed
2. **Scrapers are thin glue** → Just navigation + call parser
3. **Fixtures are truth** → Real HTML from real banks
4. **Three test modes** → Mock (CI), Replay (integration), Live (rare)
5. **Selectors are constants** → Easy to update when sites change
6. **Errors are typed** → Easy to handle in calling code

```
                    TEST COVERAGE TARGET
    ┌─────────────────────────────────────────────────┐
    │                                                 │
    │   Parsers:     95%+ coverage (unit tests)       │
    │   Scrapers:    70%+ coverage (integration)      │
    │   E2E:         Happy path only (live tests)     │
    │                                                 │
    └─────────────────────────────────────────────────┘

```

This approach gives you confidence that parsing logic is correct while minimizing flaky browser tests.