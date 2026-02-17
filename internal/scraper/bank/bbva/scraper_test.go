package bbva

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMode
type TestMode string

const (
	TestModeMock   TestMode = "mock"   // Use static fixtures
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
func TestBBVAScraper_Login_ReplaySuccess_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	// Load recorded session
	harPath := filepath.Join("testdata", "recordings", "login-success.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har, testutil.WithVerbose(true))

	t.Logf("Loaded HAR with %d entries", len(har.Entries))
	stats := replayer.Stats()
	t.Logf("Replayer stats: exact=%d, path=%d", stats["exact_matches"], stats["path_matches"])

	// Create scraper with replay hijacker
	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// Test login (credentials don't matter in replay mode)
	ctx := context.Background()
	session, err := scraper.Login(ctx, Credentials{
		CompanyCode: "test-company",
		UserCode:    "test-user",
		Password:    "test-password",
	})

	require.NoError(t, err, "Login should succeed with recorded session")
	assert.NotEmpty(t, session.ID, "Session ID should be set")
	assert.Equal(t, bank.BankBBVA, session.BankCode, "Bank code should be BBVA")
	assert.False(t, session.ExpiresAt.IsZero(), "Session expiry should be set")
}

func TestBBVAScraper_Login_ReplayError403BotDetection_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	// Load recorded error session
	harPath := filepath.Join("testdata", "recordings", "login-bot-detection.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	// Create scraper with replay hijacker
	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(30*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// Test login with (simulated) invalid credentials
	ctx := context.Background()
	session, err := scraper.Login(ctx, Credentials{
		CompanyCode: "invalid",
		UserCode:    "invalid",
		Password:    "invalid",
	})

	require.Error(t, err, "Login should fail with recorded error session")
	assert.Nil(t, session, "Session should be nil on error")

	// Verify it's a scraper error with invalid credentials cause
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr, "Error should be a ScraperError")
	assert.Equal(t, bank.BankBBVA, scraperErr.BankCode)
	assert.Equal(t, "Login", scraperErr.Operation)
}
