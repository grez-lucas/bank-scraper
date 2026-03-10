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

// requireLiveCreds reads BBVA credentials from environment variables.
// Skips the test if any are missing.
func requireLiveCreds(t *testing.T) Credentials {
	t.Helper()
	company := os.Getenv("BBVA_COMPANY_CODE")
	user := os.Getenv("BBVA_USER_CODE")
	pass := os.Getenv("BBVA_PASSWORD")
	if company == "" || user == "" || pass == "" {
		t.Skip("Skipping: requires BBVA_COMPANY_CODE, BBVA_USER_CODE, BBVA_PASSWORD env vars")
	}
	return Credentials{
		CompanyCode: company,
		UserCode:    user,
		Password:    pass,
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

	// Page lifecycle: page and session should persist after successful login
	assert.NotNil(t, scraper.page, "Page should be kept alive after successful login")
	assert.NotNil(t, scraper.session, "Session should be stored on scraper")
	assert.WithinDuration(t, time.Now().Add(bbvaSessionTimeout), session.ExpiresAt, 5*time.Second,
		"Session expiry should be ~10 minutes from now")
}

func TestBBVAScraper_Login_ReplayError403BotDetection_Integration(t *testing.T) {
	t.Skip("TODO: re-record with #enviarSenda for Senda-based bot detection")

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
		WithTimeout(10*time.Second),
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
	require.ErrorIs(t, err, bank.ErrBotDetection, "Error cause should be Bot Detection")

	assert.Equal(t, bank.BankBBVA, scraperErr.BankCode)
	assert.Equal(t, "Login", scraperErr.Operation)

	// Page lifecycle: page and session should be cleaned up after failed login
	assert.Nil(t, scraper.page, "Page should be closed after failed login")
	assert.Nil(t, scraper.session, "Session should not be stored after failed login")
}

func TestBBVAScraper_Login_ReplayErrorInvalidCredentials_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	// Load recorded error session (Senda flow: iframe gets 403 → LOGIN_ERROR → span shows error)
	harPath := filepath.Join("testdata", "recordings", "login-invalid-credentials-legacy.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	// Create scraper with replay hijacker
	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

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
	require.ErrorIs(t, err, bank.ErrInvalidCredentials, "Error cause should be Invalid Credentials")
	assert.Equal(t, bank.BankBBVA, scraperErr.BankCode)
	assert.Equal(t, "Login", scraperErr.Operation)

	// Senda probe: error text comes from direct API probe in replay mode
	assert.Contains(t, scraperErr.Details, "error-code 160",
		"Details should contain Senda API error code")

	// Page lifecycle: page and session should be cleaned up after failed login
	assert.Nil(t, scraper.page, "Page should be closed after failed login")
	assert.Nil(t, scraper.session, "Session should not be stored after failed login")
}

func TestBBVAScraper_Login_ReplayRelogin_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	harPath := filepath.Join("testdata", "recordings", "login-success.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()
	creds := Credentials{
		CompanyCode: "test-company",
		UserCode:    "test-user",
		Password:    "test-password",
	}

	// First login
	session1, err := scraper.Login(ctx, creds)
	require.NoError(t, err)
	firstPage := scraper.page

	// Second login (re-login) — should replace session and page
	session2, err := scraper.Login(ctx, creds)
	require.NoError(t, err)

	assert.NotEqual(t, session1.ID, session2.ID, "Re-login should create a new session ID")
	assert.NotNil(t, scraper.page, "Page should be alive after re-login")
	assert.NotSame(t, firstPage, scraper.page, "Re-login should create a new page")
}

func TestBBVAScraper_GetBalance_Replay_Integration(t *testing.T) {
	t.Skip("TODO: portal SPA (Cells framework) cannot initialize in replay mode — " +
		"CDP Fetch bypasses cookie/session setup needed by the Polymer web components. " +
		"Requires re-architecting replay to inject Cells session state or using a " +
		"direct API probe approach for GetBalance.")

	skipUnlessMode(t, TestModeReplay)

	// HAR must contain login + accounts page navigation traffic
	harPath := filepath.Join("testdata", "recordings", "get-balance.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(45*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()

	// Login first — hijacker stays alive for GetBalance navigation
	_, err = scraper.Login(ctx, Credentials{
		CompanyCode: "test-company",
		UserCode:    "test-user",
		Password:    "test-password",
	})
	require.NoError(t, err, "Login should succeed")

	// Act
	balances, err := scraper.GetBalance(ctx)

	require.NoError(t, err)
	require.Len(t, balances, 2)

	pen := balances[0]
	assert.Equal(t, bank.CurrencyPEN, pen.Currency)
	assert.NotEmpty(t, pen.AccountID)
	assert.Greater(t, pen.AvailableBalance, int64(0))
	assert.WithinDuration(t, time.Now(), pen.FetchedAt, 10*time.Second)

	usd := balances[1]
	assert.Equal(t, bank.CurrencyUSD, usd.Currency)
	assert.NotEmpty(t, usd.AccountID)
	assert.Greater(t, usd.AvailableBalance, int64(0))
	assert.WithinDuration(t, time.Now(), usd.FetchedAt, 10*time.Second)
}

func TestBBVAScraper_GetTransactions_Replay_Integration(t *testing.T) {
	t.Skip("TODO: portal SPA (Cells framework) cannot initialize in replay mode — " +
		"CDP Fetch bypasses cookie/session setup needed by the Polymer web components. " +
		"Requires re-architecting replay to inject Cells session state or using a " +
		"direct API probe approach for GetTransactions.")

	skipUnlessMode(t, TestModeReplay)

	// HAR must contain login + navigate to transactions page traffic
	harPath := filepath.Join("testdata", "recordings", "get-transactions.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	scraper, err := NewBBVAScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(45*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()

	// Login first
	session, err := scraper.Login(ctx, Credentials{
		CompanyCode: "test-company",
		UserCode:    "test-user",
		Password:    "test-pass",
	})
	require.NoError(t, err)
	require.NotNil(t, session)

	// Get transactions
	txns, err := scraper.GetTransactions(ctx, "PE001101190100064607")

	require.NoError(t, err)
	require.NotEmpty(t, txns)

	tx0 := txns[0]
	assert.NotEmpty(t, tx0.ID)
	assert.NotEmpty(t, tx0.Description)
	assert.NotZero(t, tx0.Amount)
	assert.False(t, tx0.Date.IsZero())
	assert.WithinDuration(t, time.Now(), tx0.Date, 365*24*time.Hour)
}

func TestClassifySendaError(t *testing.T) {
	tests := []struct {
		name      string
		errorText string
		wantErr   error
	}{
		{
			name:      "invalid credentials",
			errorText: "Es necesario que corrijas los datos que ingresaste para poder continuar.",
			wantErr:   bank.ErrInvalidCredentials,
		},
		{
			name:      "user blocked",
			errorText: "Tu usuario está bloqueado. Para desbloquearlo, comunícate con nosotros.",
			wantErr:   bank.ErrInvalidCredentials,
		},
		{
			name:      "too many attempts",
			errorText: "Los datos ingresados son incorrectos. Alcanzaste el límite de intentos.",
			wantErr:   bank.ErrInvalidCredentials,
		},
		{
			name:      "user unavailable",
			errorText: "Usuario no disponible.",
			wantErr:   bank.ErrBankUnavailable,
		},
		{
			name:      "unknown error",
			errorText: "Some unexpected error message",
			wantErr:   bank.ErrUnknown,
		},
		// Probe-format cases (from sendaAPIProbe in replay mode)
		{
			name:      "probe error-code 160",
			errorText: "senda API error-code 160",
			wantErr:   bank.ErrInvalidCredentials,
		},
		{
			name:      "probe error-code 162",
			errorText: "senda API error-code 162",
			wantErr:   bank.ErrInvalidCredentials,
		},
		{
			name:      "probe error-code unknown",
			errorText: "senda API error-code 999",
			wantErr:   bank.ErrUnknown,
		},
		{
			name:      "probe raw status",
			errorText: "senda API 500: internal server error",
			wantErr:   bank.ErrUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySendaError(tt.errorText)
			assert.ErrorIs(t, got, tt.wantErr)
		})
	}
}

func TestClassifySendaErrorCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr error
	}{
		{"160 invalid credentials", "160", bank.ErrInvalidCredentials},
		{"162 invalid credentials", "162", bank.ErrInvalidCredentials},
		{"999 unknown", "999", bank.ErrUnknown},
		{"empty unknown", "", bank.ErrUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySendaErrorCode(tt.code)
			assert.ErrorIs(t, got, tt.wantErr)
		})
	}
}

func TestBBVAScraper_Live_FullFlow(t *testing.T) {
	skipUnlessMode(t, TestModeLive)
	creds := requireLiveCreds(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	scraper, err := NewBBVAScraper(WithTimeout(60 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// --- Login ---
	session, err := scraper.Login(ctx, creds)
	require.NoError(t, err, "Login failed")
	assert.NotEmpty(t, session.ID)
	assert.Equal(t, bank.BankBBVA, session.BankCode)
	assert.False(t, session.ExpiresAt.IsZero())
	t.Logf("Login OK — session=%s expires=%s", session.ID, session.ExpiresAt.Format(time.RFC3339))

	// --- GetBalance ---
	balances, err := scraper.GetBalance(ctx)
	require.NoError(t, err, "GetBalance failed")
	require.NotEmpty(t, balances, "Expected at least one account balance")

	t.Logf("Balances (%d accounts):", len(balances))
	for i, b := range balances {
		assert.NotEmpty(t, b.AccountID, "balance[%d] AccountID should not be empty", i)
		assert.Contains(t, []bank.Currency{bank.CurrencyPEN, bank.CurrencyUSD}, b.Currency,
			"balance[%d] Currency should be PEN or USD", i)
		assert.GreaterOrEqual(t, b.AvailableBalance, int64(0),
			"balance[%d] AvailableBalance should be non-negative", i)
		assert.WithinDuration(t, time.Now(), b.FetchedAt, 30*time.Second,
			"balance[%d] FetchedAt should be recent", i)
		t.Logf("  [%d] %s %s available=%d current=%d account=%s",
			i, b.Currency, b.AccountID, b.AvailableBalance, b.CurrentBalance, b.AccountID)
	}

	// --- GetTransactions (using first account) ---
	accountID := balances[0].AccountID
	t.Logf("Fetching transactions for account %s...", accountID)

	txns, err := scraper.GetTransactions(ctx, accountID)
	require.NoError(t, err, "GetTransactions failed")
	t.Logf("Transactions: %d total", len(txns))

	if len(txns) > 0 {
		oneYearAgo := time.Now().AddDate(-1, 0, 0)
		for i, tx := range txns {
			assert.NotEmpty(t, tx.Description, "tx[%d] Description should not be empty", i)
			assert.Greater(t, tx.Amount, int64(0), "tx[%d] Amount should be positive", i)
			assert.Contains(t, []bank.TransactionType{bank.TransactionCredit, bank.TransactionDebit}, tx.Type,
				"tx[%d] Type should be CREDIT or DEBIT", i)
			assert.False(t, tx.Date.IsZero(), "tx[%d] Date should not be zero", i)
			assert.True(t, tx.Date.After(oneYearAgo), "tx[%d] Date should be within 1 year", i)
		}

		first := txns[0]
		last := txns[len(txns)-1]
		t.Logf("  First: %s %s %s %d", first.Date.Format("2006-01-02"), first.Type, first.Description, first.Amount)
		t.Logf("  Last:  %s %s %s %d", last.Date.Format("2006-01-02"), last.Type, last.Description, last.Amount)
	}
}
