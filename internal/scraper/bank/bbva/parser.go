package bbva

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
)

const (
	// -- ACCOUNTS --

	CurrencySymbolPEN = "S/"
	CurrencySymbolUSD = "$"

	CurrencyCodePEN = "PEN"
	CurrencyCodeUSD = "USD"

	// Table Labels
	LabelSoles   = "SOLES"
	LabelDollars = "DOLARES"

	// -- TRANSACTIONS --

	BBVADateLayout = "02-01-2006"
)

type LoginErrorInfo struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *LoginErrorInfo) Error() string {
	return fmt.Sprintf("[%d] (Code: %s) %s", e.HTTPStatus, e.Code, e.Message)
}

func (e *LoginErrorInfo) Unwrap() error {
	return fmt.Errorf("%s", e.Error())
}

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

// ParseAccountBalances parses the 2026 redesigned accounts page.
// Auto-detects view mode: list view (both balances) or tile view (available only).
func ParseAccountBalances(html string) ([]bank.Balance, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", bank.ErrParsingFailed, err)
	}

	// List view has more data (both available and accounted balance).
	if tables := doc.Find(SelectorAccountTable); tables.Length() > 0 {
		return parseAccountsListView(doc)
	}

	// Tile view only has available balance.
	if cards := doc.Find(SelectorAccountCard); cards.Length() > 0 {
		return parseAccountsTileView(doc)
	}

	return nil, fmt.Errorf("%w: no account elements found", bank.ErrParsingFailed)
}

func ParseTransactions(html string) ([]bank.Transaction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	// 1. Check if we got an error indicating there's no movements
	if hasNoMovements(doc) {
		return []bank.Transaction{}, nil
	}

	// 2. Parse the transactions table
	txnTable := doc.Find(SelectorTransactionsTable)
	if txnTable.Length() == 0 {
		return nil, fmt.Errorf("%w: table not found with selector: %s", bank.ErrParsingFailed, SelectorTransactionsTable)
	}

	txnRows := doc.Find(SelectorTransactionRow)

	// NOTE: If no transactions are found but the table exists,
	// we return an empty slice.
	transactions := make([]bank.Transaction, 0, txnRows.Length())

	var parseErr error
	// Iterate over rows
	txnRows.Each(func(i int, s *goquery.Selection) {
		if parseErr != nil {
			return
		}

		// 1. Parse the HTML row
		tempRow, err := parseTransactionRow(s)
		if err != nil {
			parseErr = fmt.Errorf("failed to parse row: %d: %w", i, err)
			return
		}

		// 2. Transform the data into a transaction
		transactions = append(transactions, *tempRow.ToTransaction())
	})

	if parseErr != nil {
		return nil, parseErr
	}

	return transactions, nil
}

func DetectLoginError(html string, statusCode int) error {
	// Handle HTTP errors first
	switch statusCode {
	case http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return &LoginErrorInfo{
			Message:    "Bank service temporarily unavailable or rate limited",
			HTTPStatus: statusCode,
		}

	case http.StatusForbidden:
		return &LoginErrorInfo{
			Message:    "Access forbidden - possible bot detection",
			HTTPStatus: statusCode,
		}
	}

	// Parse HTML for error details (handles 404 and 200 cases)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		// Parse error - no login error page
		return nil
	}

	code := strings.TrimSpace(doc.Find(SelectorLoginErrorCode).Text())
	msg := strings.TrimSpace(doc.Find(SelectorLoginErrorSpan).Text())
	if msg == "" {
		// Check if the message is somewhere else - perhaps we got another html doc
		msg = strings.TrimSpace(doc.Find(SelectorLoginErrorMessage).Text())
	}

	// No error found
	if code == "" && msg == "" {
		return nil
	}

	return &LoginErrorInfo{
		Code:       code,
		Message:    msg,
		HTTPStatus: statusCode,
	}
}

// --- PRIVATE DOMAIN LOGIC ---

// FIXME: REFACTOR THIS
func parseTransactionRow(s *goquery.Selection) (*BBVARow, error) {
	panic("fix me")
}

// hasNoMovements checks if the transaction page has a "No Movements" error.
func hasNoMovements(doc *goquery.Document) bool {
	table := doc.Find(SelectorTransactionsTable)

	if table.Length() == 0 {
		return false
	}

	state, found := table.Attr("state")
	if !found {
		return false
	}

	return state == "noresults"
}

// --- ACCOUNTS PAGE (2026) ---

func parseAccountsListView(doc *goquery.Document) ([]bank.Balance, error) {
	var balances []bank.Balance
	var parseErr error

	doc.Find(SelectorAccountTable).EachWithBreak(func(i int, table *goquery.Selection) bool {
		currencyCode, exists := table.Attr("list-group-currency")
		if !exists {
			parseErr = fmt.Errorf("%w: table %d missing list-group-currency", bank.ErrParsingFailed, i)
			return false
		}

		currency, err := currencyFromCode(currencyCode)
		if err != nil {
			parseErr = fmt.Errorf("%w: table %d: %v", bank.ErrParsingFailed, i, err)
			return false
		}

		table.Find(SelectorAccountRow).EachWithBreak(func(j int, row *goquery.Selection) bool {
			bal, err := parseListViewRow(row, currency)
			if err != nil {
				parseErr = fmt.Errorf("%w: table %d row %d: %v", bank.ErrParsingFailed, i, j, err)
				return false
			}
			balances = append(balances, *bal)
			return true
		})

		return parseErr == nil
	})

	if parseErr != nil {
		return nil, parseErr
	}

	return balances, nil
}

func parseListViewRow(row *goquery.Selection, currency bank.Currency) (*bank.Balance, error) {
	desc := row.Find(SelectorAccountDescription)
	accountID := desc.AttrOr("text", "")

	availStr := row.Find(SelectorAvailableBalance).AttrOr("amount", "")
	if availStr == "" {
		return nil, fmt.Errorf("missing available balance amount")
	}
	availBal, err := ParseSpanishAmount(availStr)
	if err != nil {
		return nil, fmt.Errorf("parse available balance %q: %w", availStr, err)
	}

	acctStr := row.Find(SelectorAccountedBalance).AttrOr("amount", "")
	if acctStr == "" {
		return nil, fmt.Errorf("missing accounted balance amount")
	}
	acctBal, err := ParseSpanishAmount(acctStr)
	if err != nil {
		return nil, fmt.Errorf("parse accounted balance %q: %w", acctStr, err)
	}

	return &bank.Balance{
		AccountID:        accountID,
		Currency:         currency,
		AvailableBalance: availBal,
		CurrentBalance:   acctBal,
		FetchedAt:        time.Now(),
	}, nil
}

func parseAccountsTileView(doc *goquery.Document) ([]bank.Balance, error) {
	var balances []bank.Balance
	var parseErr error

	doc.Find(SelectorAccountCard).EachWithBreak(func(i int, card *goquery.Selection) bool {
		bal, err := parseTileViewCard(card)
		if err != nil {
			parseErr = fmt.Errorf("%w: card %d: %v", bank.ErrParsingFailed, i, err)
			return false
		}
		if bal == nil {
			return true // Skip cards without amount data
		}
		balances = append(balances, *bal)
		return true
	})

	if parseErr != nil {
		return nil, parseErr
	}

	return balances, nil
}

func parseTileViewCard(card *goquery.Selection) (*bank.Balance, error) {
	accountID := card.AttrOr("id", "")

	amountStr, exists := card.Attr("product-amount")
	if !exists || amountStr == "" {
		return nil, nil // Skip cards without amount (e.g., "Todas las cuentas" overview)
	}

	currSymbol := card.AttrOr("product-amount-currency", "")
	currency, err := currencyFromSymbol(currSymbol)
	if err != nil {
		return nil, err
	}

	amount, err := ParseSpanishAmount(amountStr)
	if err != nil {
		return nil, fmt.Errorf("parse amount %q: %w", amountStr, err)
	}

	return &bank.Balance{
		AccountID:        accountID,
		Currency:         currency,
		AvailableBalance: amount,
		CurrentBalance:   0, // Tile view only shows available balance
		FetchedAt:        time.Now(),
	}, nil
}

func currencyFromSymbol(symbol string) (bank.Currency, error) {
	switch symbol {
	case CurrencySymbolPEN:
		return bank.CurrencyPEN, nil
	case CurrencySymbolUSD:
		return bank.CurrencyUSD, nil
	default:
		return "", fmt.Errorf("unknown currency symbol: %q", symbol)
	}
}

func currencyFromCode(code string) (bank.Currency, error) {
	switch code {
	case CurrencyCodePEN:
		return bank.CurrencyPEN, nil
	case CurrencyCodeUSD:
		return bank.CurrencyUSD, nil
	default:
		return "", fmt.Errorf("unknown currency code: %q", code)
	}
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
