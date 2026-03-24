package service

import (
	"context"
	"fmt"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/bbva"
)

// ScraperCredentialTester implements CredentialTester by calling the actual bank scrapers.
type ScraperCredentialTester struct{}

// NewScraperCredentialTester creates a new ScraperCredentialTester.
func NewScraperCredentialTester() *ScraperCredentialTester {
	return &ScraperCredentialTester{}
}

// TestCredentials validates bank credentials by attempting a login via the scraper.
// Launches a headless browser, logs in, then logs out and cleans up.
func (t *ScraperCredentialTester) TestCredentials(ctx context.Context, bankCode string, fields map[string]string) error {
	switch bank.Code(bankCode) {
	case bank.BankBBVA:
		return t.testBBVA(ctx, fields)
	default:
		return fmt.Errorf("unsupported bank for testing: %s", bankCode)
	}
}

func (t *ScraperCredentialTester) testBBVA(ctx context.Context, fields map[string]string) error {
	scraper, err := bbva.NewScraper(bbva.WithTimeout(60 * time.Second))
	if err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	defer func() { _ = scraper.Close() }()

	if _, err := scraper.Login(ctx, fields); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	_ = scraper.Logout(ctx)
	return nil
}
