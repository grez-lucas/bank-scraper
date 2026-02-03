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

func ParseBalances(string) (BalanceResult, error) {
	return BalanceResult{
		USD: bank.Balance{
			AccountID:        "",
			Currency:         bank.CurrencyUSD,
			AvailableBalance: 0,
			CurrentBalance:   0,
			FetchedAt:        time.Now(),
		},
		PEN: bank.Balance{
			AccountID:        "",
			Currency:         bank.CurrencyPEN,
			AvailableBalance: 0,
			CurrentBalance:   0,
			FetchedAt:        time.Now(),
		},
	}, nil
}

func ParseBalancePEN(html string) (*bank.Balance, error) {
	return extractBalance(html, LabelSoles, bank.CurrencyPEN)
}

func ParseBalanceUSD(html string) (*bank.Balance, error) {
	return extractBalance(html, LabelDollars, bank.CurrencyUSD)
}

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

	doc.Find(SelectorAccountsTable).Each(func(i int, s *goquery.Selection) {
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

	balance.CurrentBalance, err = ToCents(currStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert current balance: %w", err)
	}
	balance.AvailableBalance, err = ToCents(availStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert available balance: %w", err)
	}

	return balance, nil
}

func ToCents(s string) (int64, error) {
	cleanStr := strings.NewReplacer(",", "", " ", ".").Replace(s)

	floatVal, err := strconv.ParseFloat(cleanStr, 64)
	if err != nil {
		return 0, err
	}

	// 3. Scale by 100 and round to nearest integer to avoid float precision issues
	// Adding 0.5 before casting to int64 is a classic "round to nearest" trick
	return int64(floatVal*100 + 0.5), nil
}
