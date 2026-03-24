// Package factory creates bank-specific scraper instances.
package factory

import (
	"fmt"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/bbva"
)

// New creates a ScraperFactory that builds bank-specific scrapers.
// Currently supports BBVA only.
func New(timeout time.Duration, headless bool) bank.ScraperFactory {
	return func(bankCode bank.Code) (bank.Scraper, error) {
		switch bankCode {
		case bank.BankBBVA:
			return bbva.NewScraper(
				bbva.WithTimeout(timeout),
				bbva.WithHeadless(headless),
			)
		default:
			return nil, fmt.Errorf("unsupported bank: %s", bankCode)
		}
	}
}
