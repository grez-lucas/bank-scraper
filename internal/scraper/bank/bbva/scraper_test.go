package bbva

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/aynifx/bank-scraper/internal/scraper/testutil"
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
func requireLiveCreds(t *testing.T) map[string]string {
	t.Helper()
	company := os.Getenv("BBVA_COMPANY_CODE")
	user := os.Getenv("BBVA_USER_CODE")
	pass := os.Getenv("BBVA_PASSWORD")
	if company == "" || user == "" || pass == "" {
		t.Skip("Skipping: requires BBVA_COMPANY_CODE, BBVA_USER_CODE, BBVA_PASSWORD env vars")
	}
	return map[string]string{
		"company_code": company,
		"user_code":    user,
		"password":     pass,
	}
}

// Integration test - runs only in replay/live mode
func TestScraper_Login_ReplaySuccess_Integration(t *testing.T) {
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
	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// Test login (credentials don't matter in replay mode)
	ctx := context.Background()
	session, err := scraper.Login(ctx, map[string]string{
		"company_code": "test-company",
		"user_code":    "test-user",
		"password":     "test-password",
	})

	require.NoError(t, err, "Login should succeed with recorded session")
	assert.NotEmpty(t, session.ID, "Session ID should be set")
	assert.Equal(t, bank.BankBBVA, session.Code, "Code should be BBVA")
	assert.False(t, session.ExpiresAt.IsZero(), "Session expiry should be set")

	// Page lifecycle: page and session should persist after successful login
	assert.NotNil(t, scraper.page, "Page should be kept alive after successful login")
	assert.NotNil(t, scraper.session, "Session should be stored on scraper")
	assert.WithinDuration(t, time.Now().Add(bbvaSessionTimeout), session.ExpiresAt, 5*time.Second,
		"Session expiry should be ~10 minutes from now")
}

func TestScraper_Login_ReplayError403BotDetection_Integration(t *testing.T) {
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
	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// Test login with (simulated) invalid credentials
	ctx := context.Background()
	session, err := scraper.Login(ctx, map[string]string{
		"company_code": "invalid",
		"user_code":    "invalid",
		"password":     "invalid",
	})

	require.Error(t, err, "Login should fail with recorded error session")
	assert.Nil(t, session, "Session should be nil on error")

	// Verify it's a scraper error with invalid credentials cause
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr, "Error should be a ScraperError")
	require.ErrorIs(t, err, bank.ErrBotDetection, "Error cause should be Bot Detection")

	assert.Equal(t, bank.BankBBVA, scraperErr.Code)
	assert.Equal(t, "Login", scraperErr.Operation)

	// Page lifecycle: page and session should be cleaned up after failed login
	assert.Nil(t, scraper.page, "Page should be closed after failed login")
	assert.Nil(t, scraper.session, "Session should not be stored after failed login")
}

func TestScraper_Login_ReplayErrorInvalidCredentials_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	// Load recorded error session (Senda flow: iframe gets 403 → LOGIN_ERROR → span shows error)
	harPath := filepath.Join("testdata", "recordings", "login-invalid-credentials-legacy.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	// Create scraper with replay hijacker
	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()
	session, err := scraper.Login(ctx, map[string]string{
		"company_code": "invalid",
		"user_code":    "invalid",
		"password":     "invalid",
	})

	require.Error(t, err, "Login should fail with recorded error session")
	assert.Nil(t, session, "Session should be nil on error")

	// Verify it's a scraper error with invalid credentials cause
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr, "Error should be a ScraperError")
	require.ErrorIs(t, err, bank.ErrInvalidCredentials, "Error cause should be Invalid Credentials")
	assert.Equal(t, bank.BankBBVA, scraperErr.Code)
	assert.Equal(t, "Login", scraperErr.Operation)

	// Senda probe: error text comes from direct API probe in replay mode
	assert.Contains(t, scraperErr.Details, "error-code 160",
		"Details should contain Senda API error code")

	// Page lifecycle: page and session should be cleaned up after failed login
	assert.Nil(t, scraper.page, "Page should be closed after failed login")
	assert.Nil(t, scraper.session, "Session should not be stored after failed login")
}

func TestScraper_Login_ReplayRelogin_Integration(t *testing.T) {
	skipUnlessMode(t, TestModeReplay)

	harPath := filepath.Join("testdata", "recordings", "login-success.har.json")
	if _, err := os.Stat(harPath); os.IsNotExist(err) {
		t.Skipf("Recording not found: %s\n", harPath)
	}

	har := testutil.MustLoadHAR(t, harPath)
	replayer := testutil.NewReplayer(har)

	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()
	creds := map[string]string{
		"company_code": "test-company",
		"user_code":    "test-user",
		"password":     "test-password",
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

func TestScraper_GetBalance_Replay_Integration(t *testing.T) {
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

	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(45*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()

	// Login first — hijacker stays alive for GetBalance navigation
	_, err = scraper.Login(ctx, map[string]string{
		"company_code": "test-company",
		"user_code":    "test-user",
		"password":     "test-password",
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

func TestScraper_GetTransactions_Replay_Integration(t *testing.T) {
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

	scraper, err := NewScraper(
		WithHijacker(replayer.Middleware()),
		WithTimeout(45*time.Second),
	)
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	ctx := context.Background()

	// Login first
	session, err := scraper.Login(ctx, map[string]string{
		"company_code": "test-company",
		"user_code":    "test-user",
		"password":     "test-pass",
	})
	require.NoError(t, err)
	require.NotNil(t, session)

	// Get transactions
	txns, err := scraper.GetTransactions(ctx, "PE001101190100064607", 50)

	require.NoError(t, err)
	require.NotEmpty(t, txns)

	tx0 := txns[0]
	assert.NotEmpty(t, tx0.ID)
	assert.NotEmpty(t, tx0.Description)
	assert.NotZero(t, tx0.Amount)
	assert.False(t, tx0.Date.IsZero())
	assert.WithinDuration(t, time.Now(), tx0.Date, 365*24*time.Hour)
}

func TestScraper_Logout_NoSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires browser")
	}

	scraper, err := NewScraper(WithTimeout(5 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	err = scraper.Logout(context.Background())

	require.Error(t, err)
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr)
	assert.Equal(t, "Logout", scraperErr.Operation)
	assert.Equal(t, bank.BankBBVA, scraperErr.Code)
	require.ErrorIs(t, err, bank.ErrSessionExpired)
}

func TestScraper_GetBalance_NoSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires browser")
	}

	scraper, err := NewScraper(WithTimeout(5 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	_, err = scraper.GetBalance(context.Background())

	require.Error(t, err)
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr)
	assert.Equal(t, "GetBalance", scraperErr.Operation)
	require.ErrorIs(t, err, bank.ErrSessionExpired)
}

func TestScraper_GetTransactions_NoSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires browser")
	}

	scraper, err := NewScraper(WithTimeout(5 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	_, err = scraper.GetTransactions(context.Background(), "any-account", 50)

	require.Error(t, err)
	var scraperErr *bank.ScraperError
	require.ErrorAs(t, err, &scraperErr)
	assert.Equal(t, "GetTransactions", scraperErr.Operation)
	require.ErrorIs(t, err, bank.ErrSessionExpired)
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

// loginAndGetAccounts creates a new scraper, logs in, and fetches balances.
// Returns the scraper (still logged in) and the list of balances.
// The caller is responsible for closing the scraper.
func loginAndGetAccounts(t *testing.T, ctx context.Context) (*Scraper, []bank.Balance) {
	t.Helper()
	creds := requireLiveCreds(t)

	scraper, err := NewScraper(WithTimeout(60 * time.Second))
	require.NoError(t, err)

	session, err := scraper.Login(ctx, creds)
	require.NoError(t, err, "Login failed")
	require.NotEmpty(t, session.ID)
	t.Logf("Login OK — session=%s expires=%s", session.ID, session.ExpiresAt.Format(time.RFC3339))

	balances, err := scraper.GetBalance(ctx)
	require.NoError(t, err, "GetBalance failed")
	require.GreaterOrEqual(t, len(balances), 2, "Expected at least 2 accounts")

	for i, b := range balances {
		t.Logf("  [%d] %s %s available=%d current=%d",
			i, b.Currency, b.AccountID, b.AvailableBalance, b.CurrentBalance)
	}

	return scraper, balances
}

// assertTransactions validates a slice of transactions.
func assertTransactions(t *testing.T, txns []bank.Transaction) {
	t.Helper()
	require.NotEmpty(t, txns, "Expected at least one transaction")

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
	t.Logf("  %d transactions", len(txns))
	t.Logf("  First: %s %s %s %d", first.Date.Format("2006-01-02"), first.Type, first.Description, first.Amount)
	t.Logf("  Last:  %s %s %s %d", last.Date.Format("2006-01-02"), last.Type, last.Description, last.Amount)
}

func TestScraper_Live_Login(t *testing.T) {
	skipUnlessMode(t, TestModeLive)
	creds := requireLiveCreds(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	scraper, err := NewScraper(WithTimeout(60 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	session, err := scraper.Login(ctx, creds)
	require.NoError(t, err, "Login failed")
	assert.NotEmpty(t, session.ID)
	assert.Equal(t, bank.BankBBVA, session.Code)
	assert.False(t, session.ExpiresAt.IsZero())
	t.Logf("Login OK — session=%s expires=%s", session.ID, session.ExpiresAt.Format(time.RFC3339))
}

func TestScraper_Live_GetBalance(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	scraper, balances := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	require.GreaterOrEqual(t, len(balances), 2, "Expected at least 2 accounts")
	for i, b := range balances {
		assert.NotEmpty(t, b.AccountID, "balance[%d] AccountID should not be empty", i)
		assert.Contains(t, []bank.Currency{bank.CurrencyPEN, bank.CurrencyUSD}, b.Currency,
			"balance[%d] Currency should be PEN or USD", i)
		assert.GreaterOrEqual(t, b.AvailableBalance, int64(0),
			"balance[%d] AvailableBalance should be non-negative", i)
		assert.WithinDuration(t, time.Now(), b.FetchedAt, 30*time.Second,
			"balance[%d] FetchedAt should be recent", i)
	}
}

func TestScraper_Live_GetTransactions_FirstAccount(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	scraper, balances := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	accountID := balances[0].AccountID
	t.Logf("Fetching transactions for first account %s (%s)...", accountID, balances[0].Currency)

	txns, err := scraper.GetTransactions(ctx, accountID, 200)
	require.NoError(t, err, "GetTransactions failed for account %s", accountID)
	assertTransactions(t, txns)
}

func TestScraper_Live_Logout(t *testing.T) {
	skipUnlessMode(t, TestModeLive)
	creds := requireLiveCreds(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	scraper, err := NewScraper(WithTimeout(60 * time.Second))
	require.NoError(t, err)
	defer func() { _ = scraper.Close() }()

	// Login
	session, err := scraper.Login(ctx, creds)
	require.NoError(t, err, "Login failed")
	require.NotEmpty(t, session.ID)
	t.Logf("Login OK — session=%s", session.ID)

	// Logout
	err = scraper.Logout(ctx)
	require.NoError(t, err, "Logout failed")
	t.Log("Logout OK")

	// Verify session state is cleared
	assert.Nil(t, scraper.page, "Page should be nil after logout")
	assert.Nil(t, scraper.session, "Session should be nil after logout")

	// Verify GetBalance returns ErrSessionExpired after logout
	_, err = scraper.GetBalance(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, bank.ErrSessionExpired)
	t.Log("GetBalance correctly returns ErrSessionExpired after logout")
}

func TestScraper_Live_GetTransactions_SecondAccount(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	scraper, balances := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	accountID := balances[1].AccountID
	t.Logf("Fetching transactions for second account %s (%s)...", accountID, balances[1].Currency)

	txns, err := scraper.GetTransactions(ctx, accountID, 200)
	require.NoError(t, err, "GetTransactions failed for account %s", accountID)
	assertTransactions(t, txns)
}

// --- Session Reuse Live Tests ---
// These tests validate that chaining multiple operations on a single session works correctly.
// The key concern is that FlattenShadowDOM mutates the live DOM, and navigateTo with cache-busting
// must fully reset the page state between operations.

func TestScraper_Live_SessionReuse_DoubleGetBalance(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	scraper, balances1 := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	t.Log("Calling GetBalance a second time on the same session...")
	balances2, err := scraper.GetBalance(ctx)
	require.NoError(t, err, "Second GetBalance failed")
	require.Len(t, balances2, len(balances1), "Account count should be consistent")

	for i := range balances1 {
		assert.Equal(t, balances1[i].AccountID, balances2[i].AccountID,
			"Account[%d] ID should match across calls", i)
		assert.Equal(t, balances1[i].Currency, balances2[i].Currency,
			"Account[%d] Currency should match across calls", i)
	}

	for i, b := range balances2 {
		t.Logf("  [%d] %s %s available=%d current=%d",
			i, b.Currency, b.AccountID, b.AvailableBalance, b.CurrentBalance)
	}
}

func TestScraper_Live_SessionReuse_BalanceAfterTransactions(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	scraper, balances1 := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	// GetTransactions for first account
	accountID := balances1[0].AccountID
	t.Logf("GetTransactions for %s (%s)...", accountID, balances1[0].Currency)
	txns, err := scraper.GetTransactions(ctx, accountID, 50)
	require.NoError(t, err, "GetTransactions failed")
	assertTransactions(t, txns)

	// GetBalance again — navigates back to accounts page
	t.Log("Calling GetBalance after GetTransactions...")
	balances2, err := scraper.GetBalance(ctx)
	require.NoError(t, err, "GetBalance after GetTransactions failed")
	require.Len(t, balances2, len(balances1), "Account count should be consistent")

	for i := range balances1 {
		assert.Equal(t, balances1[i].AccountID, balances2[i].AccountID,
			"Account[%d] ID should match after round-trip", i)
		assert.Equal(t, balances1[i].Currency, balances2[i].Currency,
			"Account[%d] Currency should match after round-trip", i)
	}
}

func TestScraper_Live_SessionReuse_TransactionsDifferentAccounts(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	scraper, balances := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()
	require.GreaterOrEqual(t, len(balances), 2, "Need at least 2 accounts")

	// GetTransactions for first account
	acct1 := balances[0].AccountID
	t.Logf("GetTransactions for account 1: %s (%s)...", acct1, balances[0].Currency)
	txns1, err := scraper.GetTransactions(ctx, acct1, 50)
	require.NoError(t, err, "GetTransactions(acct1) failed")
	assertTransactions(t, txns1)

	// GetTransactions for second account — switches accounts within same session
	acct2 := balances[1].AccountID
	t.Logf("GetTransactions for account 2: %s (%s)...", acct2, balances[1].Currency)
	txns2, err := scraper.GetTransactions(ctx, acct2, 50)
	require.NoError(t, err, "GetTransactions(acct2) failed")
	assertTransactions(t, txns2)

	// Warn if first transactions are identical (would indicate stale SPA state)
	if len(txns1) > 0 && len(txns2) > 0 &&
		txns1[0].Description == txns2[0].Description &&
		txns1[0].Amount == txns2[0].Amount &&
		txns1[0].Date.Equal(txns2[0].Date) {
		t.Log("WARNING: First transactions from both accounts are identical — possible stale SPA state")
	}
}

func TestScraper_Live_SessionReuse_TransactionsSameAccountTwice(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	scraper, balances := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()

	accountID := balances[0].AccountID
	t.Logf("GetTransactions (call 1) for %s...", accountID)
	txns1, err := scraper.GetTransactions(ctx, accountID, 50)
	require.NoError(t, err, "First GetTransactions failed")
	assertTransactions(t, txns1)

	t.Logf("GetTransactions (call 2) for same account %s...", accountID)
	txns2, err := scraper.GetTransactions(ctx, accountID, 50)
	require.NoError(t, err, "Second GetTransactions failed")
	assertTransactions(t, txns2)

	// Idempotency: same account should return same transaction count and data
	assert.Equal(t, len(txns1), len(txns2), "Transaction count should be identical for same account")
	if len(txns1) > 0 && len(txns2) > 0 {
		assert.Equal(t, txns1[0].Description, txns2[0].Description,
			"First transaction description should match")
		assert.Equal(t, txns1[0].Amount, txns2[0].Amount,
			"First transaction amount should match")
		last1 := txns1[len(txns1)-1]
		last2 := txns2[len(txns2)-1]
		assert.Equal(t, last1.Description, last2.Description,
			"Last transaction description should match")
		assert.Equal(t, last1.Amount, last2.Amount,
			"Last transaction amount should match")
	}
}

func TestScraper_Live_SessionReuse_FullWorkflow(t *testing.T) {
	skipUnlessMode(t, TestModeLive)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	scraper, balances1 := loginAndGetAccounts(t, ctx)
	defer func() { _ = scraper.Close() }()
	require.GreaterOrEqual(t, len(balances1), 2, "Need at least 2 accounts")

	// Step 1: GetTransactions for first account
	acct1 := balances1[0].AccountID
	t.Logf("Step 1: GetTransactions for %s (%s)...", acct1, balances1[0].Currency)
	txns1, err := scraper.GetTransactions(ctx, acct1, 50)
	require.NoError(t, err, "GetTransactions(acct1) failed")
	assertTransactions(t, txns1)

	// Step 2: GetTransactions for second account
	acct2 := balances1[1].AccountID
	t.Logf("Step 2: GetTransactions for %s (%s)...", acct2, balances1[1].Currency)
	txns2, err := scraper.GetTransactions(ctx, acct2, 50)
	require.NoError(t, err, "GetTransactions(acct2) failed")
	assertTransactions(t, txns2)

	// Step 3: GetBalance again — full round-trip back to accounts page
	t.Log("Step 3: GetBalance after two GetTransactions calls...")
	balances2, err := scraper.GetBalance(ctx)
	require.NoError(t, err, "Final GetBalance failed")
	require.Len(t, balances2, len(balances1), "Account count should be consistent")
	for i := range balances1 {
		assert.Equal(t, balances1[i].AccountID, balances2[i].AccountID,
			"Account[%d] ID should match after full workflow", i)
		assert.Equal(t, balances1[i].Currency, balances2[i].Currency,
			"Account[%d] Currency should match after full workflow", i)
	}

	// Step 4: Logout
	t.Log("Step 4: Logout...")
	err = scraper.Logout(ctx)
	require.NoError(t, err, "Logout failed")
	assert.Nil(t, scraper.page, "Page should be nil after logout")
	assert.Nil(t, scraper.session, "Session should be nil after logout")
	t.Log("Logout OK")

	// Step 5: Verify session is invalidated
	_, err = scraper.GetBalance(ctx)
	require.Error(t, err, "GetBalance should fail after logout")
	require.ErrorIs(t, err, bank.ErrSessionExpired, "Should get ErrSessionExpired after logout")
	t.Log("Post-logout GetBalance correctly returns ErrSessionExpired")
}
