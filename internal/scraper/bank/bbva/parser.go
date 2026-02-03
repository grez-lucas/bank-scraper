package bbva

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
)

const (
	// Table Column Indexes
	colAccountID        = 0
	colCurrentBalance   = 2
	colAvailableBalance = 3
	colCurrency         = 4

	// Table Labels
	LabelSoles   = "SOLES"
	LabelDollars = "DOLARES"
)

type BalanceResult struct {
	USD bank.Balance
	PEN bank.Balance
}

// BBVARow represents a row in the BBVA transactions HTML table
type BBVARow struct {
	FOperacion time.Time
	FValor     time.Time
	Codigo     int8
	NDoc       int
	Concepto   string
	Importe    int64 // Can be negative, represents two decimal precision (e.g., -2,000.00 or -0.90)
	Oficina    int
}

// --- PUBLIC API ---

func ParseBalances(html string) (*BalanceResult, error) {
	penBalance, err := ParseBalancePEN(html)
	if err != nil {
		return nil, err
	}
	usdBalance, err := ParseBalanceUSD(html)
	if err != nil {
		return nil, err
	}
	return &BalanceResult{
		USD: *usdBalance,
		PEN: *penBalance,
	}, nil
}

func ParseBalancePEN(html string) (*bank.Balance, error) {
	return extractBalance(html, LabelSoles, bank.CurrencyPEN)
}

func ParseBalanceUSD(html string) (*bank.Balance, error) {
	return extractBalance(html, LabelDollars, bank.CurrencyUSD)
}

func ParseTransactions(html string) ([]bank.Transaction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var rowCounter int
	var transactions []bank.Transaction
	// Select all rows of the table
	doc.Find(SelectorTransactionsTableRows).Each(func(i int, s *goquery.Selection) {
		rowCounter++
	})
	fmt.Printf("Found %d rows!\n", rowCounter)
	transactions = make([]bank.Transaction, rowCounter)

	return transactions, nil
}

// --- PRIVATE DOMAIN LOGIC ---

func extractBalance(html string, targetLabel string, currency bank.Currency) (*bank.Balance, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse HTML: %v", bank.ErrParsingFailed, err)
	}

	var found bool
	balance := &bank.Balance{
		AccountID:        "",
		Currency:         currency,
		AvailableBalance: 0,
		CurrentBalance:   0,
		FetchedAt:        time.Now(),
	}

	var availStr, currStr string

	doc.Find(SelectorAccountsTableRows).Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		rowCurrency := strings.TrimSpace(cells.Eq(colCurrency).Text())

		if rowCurrency == targetLabel {
			found = true
			balance.AccountID = strings.TrimSpace(cells.Eq(colAccountID).Text())
			currStr = strings.TrimSpace(cells.Eq(colCurrentBalance).Text())
			availStr = strings.TrimSpace(cells.Eq(colAvailableBalance).Text())
		}
	})

	if !found {
		return nil, fmt.Errorf("%w: %s balance row not found", bank.ErrParsingFailed, currency)
	}

	balance.CurrentBalance, err = ParseSpanishAmount(currStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert current balance: %w", err)
	}
	balance.AvailableBalance, err = ParseSpanishAmount(availStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert available balance: %w", err)
	}

	return balance, nil
}

// --- LOW LEVEL UTILITIES ---

// ParseSpanishAmount transforms a string representation of a number to an int64
// representation with two decimals.
func ParseSpanishAmount(s string) (int64, error) {
	cleanStr := strings.NewReplacer(",", "", " ", "").Replace(s)

	floatVal, err := strconv.ParseFloat(cleanStr, 64)
	if err != nil {
		return 0, err
	}

	// 3. Scale by 100 and round to nearest integer to avoid float precision issues
	// Adding 0.5 before casting to int64 is a classic "round to nearest" trick
	return int64(floatVal*100 + 0.5), nil
}
