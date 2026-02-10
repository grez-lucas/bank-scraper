package bbva

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
)

const (
	// -- ACCOUNTS --
	// Table Column Indexes
	colAccountID        = 0
	colCurrentBalance   = 2
	colAvailableBalance = 3
	colCurrency         = 4

	// Table Labels
	LabelSoles   = "SOLES"
	LabelDollars = "DOLARES"

	// -- TRANSACTIONS --
	colFOperacion = 0
	colFValor     = 1
	colCodigo     = 2
	colNumDoc     = 3
	colConcepto   = 4
	colImporte    = 5
	colOficina    = 6

	BBVADateLayout = "02-01-2006"
)

type BalanceResult struct {
	USD bank.Balance
	PEN bank.Balance
}

// BBVARow represents a row in the BBVA transactions HTML table
type BBVARow struct {
	FOperacion time.Time
	FValor     time.Time
	Codigo     string
	NDoc       string
	Concepto   string
	Importe    int64 // Can be negative, represents two decimal precision (e.g., -2,000.00 or -0.90)
	Oficina    string
}

func (r *BBVARow) IsPositiveImport() bool {
	return r.Importe > 0
}

// ToTransaction transforms a BBVA Row into a standard transaction
func (r *BBVARow) ToTransaction() *bank.Transaction {
	absAmount := r.Importe
	txnType := bank.TransactionCredit

	if !r.IsPositiveImport() {
		txnType = bank.TransactionDebit
		absAmount = -absAmount
	}

	// TODO: Implement ME
	return &bank.Transaction{
		ID:           r.NDoc,
		Reference:    "",
		Date:         r.FOperacion,
		ValueDate:    r.FValor,
		Description:  r.Concepto,
		Amount:       absAmount,
		Type:         txnType,
		BalanceAfter: nil,
		Extra:        map[string]string{"Office": r.Oficina, "Codigo": r.Codigo},
	}
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

	txnTable := doc.Find(SelectorTransactionsTable)
	if txnTable.Length() == 0 {
		return nil, fmt.Errorf("%w: table not found with selector: %s", bank.ErrParsingFailed, SelectorTransactionsTable)
	}

	txnRows := doc.Find(SelectorTransactionsTableRows)

	// NOTE: If no transactions are found but the table exists,
	// we return an empty slice.
	transactions := make([]bank.Transaction, txnRows.Length())

	// Iterate over rows
	txnRows.Each(func(i int, s *goquery.Selection) {
		// 1. Parse the HTML row
		tempRow, err := parseTransactionRow(s)
		if err != nil {
			return
		}

		// 2. Transform the data into a transaction
		transactions[i] = *tempRow.ToTransaction()
	})

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

func parseTransactionRow(s *goquery.Selection) (*BBVARow, error) {
	var err error
	var row BBVARow
	if s == nil {
		return nil, fmt.Errorf("unable to parse nil selection")
	}
	cells := s.Find("td")

	row.FOperacion, err = ParseBankDate(cells.Eq(colFOperacion).Text())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse Fecha de Operacion: %v", bank.ErrParsingFailed, err)
	}
	row.FValor, err = ParseBankDate(cells.Eq(colFValor).Text())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse Fecha de Operacion: %v", bank.ErrParsingFailed, err)
	}
	row.Codigo = strings.TrimSpace(cells.Eq(colCodigo).Text())
	row.NDoc = strings.TrimSpace(cells.Eq(colNumDoc).Text())
	row.Concepto = strings.TrimSpace(cells.Eq(colConcepto).Text())

	row.Importe, err = ParseSpanishAmount(cells.Eq(colImporte).Text())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse Importe: %v", bank.ErrParsingFailed, err)
	}

	row.Oficina = strings.TrimSpace(cells.Eq(colOficina).Text())

	return &row, nil
}

// --- LOW LEVEL UTILITIES ---

// ParseSpanishAmount transforms a string representation of a number to an int64
// representation with two decimals.
func ParseSpanishAmount(s string) (int64, error) {
	cleanStr := strings.NewReplacer(",", "", " ", "").Replace(s)

	// Parse into float
	floatVal, err := strconv.ParseFloat(cleanStr, 64)
	if err != nil {
		return 0, err
	}

	// Round
	return int64(math.Round(floatVal * 100)), nil
}

func ParseBankDate(s string) (time.Time, error) {
	// 1. Clean up the string
	cleanStr := strings.TrimSpace(s)
	// 2. Parse using reference layout
	t, err := time.Parse(BBVADateLayout, cleanStr)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
