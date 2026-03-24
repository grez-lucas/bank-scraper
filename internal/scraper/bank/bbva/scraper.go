// Package bbva defines the scraper and parsing logic to process the BBVA
// portal.
package bbva

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/browser"
	"github.com/grez-lucas/bank-scraper/internal/scraper/debug"
)

const (
	baseURL     = "https://www.bbvanetcash.pe"
	loginURL    = baseURL + "/DFAUTH85/mult/KDPOSolicitarCredenciales_es.html"
	portalURL   = baseURL + "/nextgenempresas/portal/index.html"
	accountsURL = baseURL + "/nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution"

	maxPaginationClicks    = 10 // Safety limit for "Ver más" pagination loop
	maxAccountsNavAttempts = 3  // Total attempts for navigate+wait on accounts page

	defaultTimeout         = 30 * time.Second
	accountsNavStepTimeout = 15 * time.Second // Timeout per step (navigate or wait) when retrying

	bbvaSessionTimeout = 10 * time.Minute

	minTransactionCount = 50
	maxTransactionCount = 250
)

const debugBaseDir = "bbva-debug"

// Scraper implements browser automation for the BBVA Net Cash portal.
type Scraper struct {
	browser  *rod.Browser
	page     *rod.Page            // Authenticated page, kept alive between operations
	router   *rod.HijackRouter    // Request hijacker, kept alive with the page
	session  *bank.Session
	debug    *debug.Collector     // Session-scoped artifact capture; nil before Login
	timeout  time.Duration
	headless bool                 // Whether to launch browser in headless mode
	hijacker func(*rod.Hijack)    // Optional hijacker for replay testing
	logger   *slog.Logger
}

// credentials holds BBVA login fields (internal, mapped from generic map).
type credentials struct {
	companyCode string
	userCode    string
	password    string
}

// Credential field keys expected in the Login credentials map.
const (
	fieldCompanyCode = "company_code"
	fieldUserCode    = "user_code"
	fieldPassword    = "password"
)

func credentialsFromMap(fields map[string]string) (credentials, error) {
	c := credentials{
		companyCode: fields[fieldCompanyCode],
		userCode:    fields[fieldUserCode],
		password:    fields[fieldPassword],
	}
	if c.companyCode == "" {
		return credentials{}, fmt.Errorf("missing required field %q", fieldCompanyCode)
	}
	if c.userCode == "" {
		return credentials{}, fmt.Errorf("missing required field %q", fieldUserCode)
	}
	if c.password == "" {
		return credentials{}, fmt.Errorf("missing required field %q", fieldPassword)
	}
	return c, nil
}

// Ensure Scraper satisfies the bank.Scraper interface.
var _ bank.Scraper = (*Scraper)(nil)

// Option pattern for configuration
type Option func(*Scraper)

// WithTimeout sets the scraper operation timeout.
func WithTimeout(d time.Duration) Option {
	return func(s *Scraper) {
		s.timeout = d
	}
}

// WithHeadless controls whether the browser launches in headless mode.
// Default is true. Set to false for visual debugging of live sessions.
func WithHeadless(headless bool) Option {
	return func(s *Scraper) {
		s.headless = headless
	}
}

// WithHijacker sets a custom hijacker middleware for request interception.
// This is used for replay testing to serve recorded responses instead of
// making real network requests.
func WithHijacker(middleware func(*rod.Hijack)) Option {
	return func(s *Scraper) {
		s.hijacker = middleware
	}
}

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Scraper) {
		s.logger = l
	}
}

// NewScraper creates a new BBVA scraper with the given options.
func NewScraper(opts ...Option) (*Scraper, error) {
	s := &Scraper{
		timeout:  defaultTimeout,
		headless: true,
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	// Launch with stealth flags to avoid bot detection
	url, err := launcher.New().
		Set("disable-blink-features", "AutomationControlled").
		Headless(s.headless).
		Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	bro := rod.New().ControlURL(url)
	if err := bro.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	s.browser = bro
	return s, nil
}

// Login authenticates with BBVA and returns a session.
// Expected credential fields: "company_code", "user_code", "password".
func (s *Scraper) Login(ctx context.Context, fields map[string]string) (*bank.Session, error) {
	op := debug.StartOp(s.logger, "Login")

	creds, err := credentialsFromMap(fields)
	if err != nil {
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrInvalidCredentials,
			Details:   err.Error(),
		}
	}

	// Close previous page if re-logging in
	if s.page != nil {
		s.stopHijacker()
		_ = s.page.Close()
		s.page = nil
		s.session = nil
		s.debug = nil
	}

	page, err := s.browser.Page(proto.TargetCreateTarget{URL: loginURL})
	if err != nil {
		op.Error("page creation failed", err)
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   err.Error(),
		}
	}

	// Close page and hijacker on failure, keep alive on success
	success := false
	defer func() {
		if !success {
			s.stopHijacker()
			_ = page.Close()
		}
	}()

	// Set up request hijacking on the base page (no timeout context).
	// The router's event context derives from the page's context at creation time.
	router := page.HijackRequests()

	if s.hijacker != nil {
		router.MustAdd("*", s.hijacker)
	} else {
		router.MustAdd("*", func(h *rod.Hijack) {
			_ = h.LoadResponse(http.DefaultClient, true)
		})
	}

	go router.Run()
	s.router = router

	// Navigation phase: load page + fill form + click login
	navCtx, navCancel := context.WithTimeout(ctx, s.timeout)
	defer navCancel()
	p := page.Context(navCtx)

	// Wait for the login form
	if err := p.WaitLoad(); err != nil {
		op.Error("page load failed", err)
		return nil, fmt.Errorf("login page load failed: %w", err)
	}

	// 1. Fill credentials (form is on main page, not in iframe)
	// Use human-like typing in live mode to avoid bot detection
	typeFn := browser.TypeFast
	if s.hijacker == nil {
		typeFn = browser.TypeHuman
	}
	if err := fillLoginForm(p, creds, typeFn); err != nil {
		op.Error("fill form failed", err)
		return nil, err
	}

	// Small random delay before clicking to appear more human-like
	if s.hijacker == nil {
		time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
	}

	// 2. Click login (#enviarSenda → Senda flow via postMessage to iframe)
	loginBtn, err := p.Element(SelectorLoginButton)
	if err != nil {
		op.Error("login button not found", err)
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("login button not found: %v", err),
		}
	}
	if err := loginBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		op.Error("click login failed", err)
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("failed to click login: %v", err),
		}
	}
	navCancel() // Navigation phase complete

	// 3. Wait for Senda outcome: portal redirect (success) or error span (failure)
	// Each wait function derives its own context from ctx.
	result := s.waitForLoginOutcome(ctx, page)
	switch result.outcome {
	case loginSuccess:
		if s.hijacker == nil {
			// Live mode: wait for the SPA to set the dashboard route hash
			if !s.waitForDashboard(ctx, page) {
				// Pre-session failure: use temporary collector
				preDebug := debug.New(debugDir(), fmt.Sprintf("pre-login-%d", time.Now().UnixNano()), s.logger)
				pageURL, dir := preDebug.Snapshot(page, "Login", "dashboard-not-loaded")
				op.Error("dashboard did not load", bank.ErrUnknown,
					slog.String("url", pageURL), slog.String("debug_dir", dir))
				return nil, &bank.ScraperError{
					Code:      bank.BankBBVA,
					Operation: "Login",
					Cause:     bank.ErrUnknown,
					Details:   fmt.Sprintf("login completed but dashboard did not load (url=%s, debug=%s)", pageURL, dir),
				}
			}
			dismissAnnouncementModal(ctx, page)
		}

	case loginError:
		op.Error("login rejected", classifySendaError(result.errorText),
			slog.String("error_text", result.errorText))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     classifySendaError(result.errorText),
			Details:   result.errorText,
		}

	case loginTimeout:
		// Pre-session failure: use temporary collector
		preDebug := debug.New(debugDir(), fmt.Sprintf("pre-login-%d", time.Now().UnixNano()), s.logger)
		pageURL, dir := preDebug.Snapshot(page, "Login", "timeout")
		op.Error("timed out waiting for redirect or error", bank.ErrUnknown,
			slog.String("url", pageURL), slog.String("debug_dir", dir))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("login timed out waiting for redirect or error (url=%s, debug=%s)", pageURL, dir),
		}
	}

	// Store session and page for subsequent operations
	session := &bank.Session{
		ID:        generateSessionID(),
		Code:      bank.BankBBVA,
		ExpiresAt: time.Now().Add(bbvaSessionTimeout),
	}
	s.session = session
	s.page = page
	s.debug = debug.New(debugDir(), session.ID, s.logger)
	s.logger = s.logger.With(slog.String("session_id", session.ID))
	success = true

	op.Success()
	return session, nil
}

// Close shuts down the browser and releases resources.
func (s *Scraper) Close() error {
	s.stopHijacker()
	if s.page != nil {
		_ = s.page.Close()
		s.page = nil
	}
	s.session = nil
	s.debug = nil
	if s.browser != nil {
		return s.browser.Close()
	}
	return nil
}

func (s *Scraper) stopHijacker() {
	if s.router != nil {
		_ = s.router.Stop()
		s.router = nil
	}
}

// Logout performs a clean logout from the BBVA portal by clicking the
// sidebar "Salir" button, confirming the logout modal, and waiting for
// the page to redirect away from the portal. After a successful logout,
// the session and page are cleared; subsequent GetBalance/GetTransactions
// calls will return ErrSessionExpired.
func (s *Scraper) Logout(ctx context.Context) error {
	op := debug.StartOp(s.logger, "Logout")

	if s.page == nil {
		return &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Logout",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	logoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	page := s.page.Context(logoutCtx)

	// Step 0: Dismiss the announcement modal if present
	dismissAnnouncementModal(logoutCtx, page)

	// Step 1: Click "Salir" button (inside nested shadow DOM)
	if !browser.DeepQueryClick(page, SelectorLogoutButton) {
		op.Error("salir button not found", bank.ErrUnknown)
		return &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Logout",
			Cause:     bank.ErrUnknown,
			Details:   "could not find or click 'Salir' button",
		}
	}
	op.Info("clicked 'Salir' button")

	// Step 2: Wait for confirmation modal to become visible
	if !waitForLogoutModal(logoutCtx, page) {
		op.Error("modal did not appear", bank.ErrUnknown)
		return &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Logout",
			Cause:     bank.ErrUnknown,
			Details:   "logout confirmation modal did not appear",
		}
	}
	op.Info("logout modal visible")

	// Step 3: Click "Cerrar sesión" confirm button inside the modal.
	//
	// The button is a Polymer web component (.action-btn). Calling .click()
	// via deepQuery triggers the full logout flow: the Polymer handler sends
	// a DELETE to grantingTicket/V02 (server-side session invalidation) plus
	// cleanup requests (campaigns, session-records).
	//
	// In headless Chrome the Cells framework's redirect after the DELETE
	// doesn't fire reliably, so we navigate to the login page ourselves.
	time.Sleep(500 * time.Millisecond) // let modal CSS animation complete

	op.Info("clicking confirm button")

	clickJS := fmt.Sprintf(`(function() {
		%s
		var modal = deepQuery(document, '%s');
		if (!modal) return 'modal not found';
		var btn = deepQuery(modal, '.action-btn');
		if (!btn) return 'button not found';
		btn.click();
		return 'clicked';
	})()`, browser.DeepQueryJS, SelectorLogoutModal)

	clickResult, clickErr := proto.RuntimeEvaluate{
		Expression:    clickJS,
		ReturnByValue: true,
	}.Call(page)
	if clickErr != nil {
		op.Error("confirm button eval failed", clickErr)
		return &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Logout",
			Cause:     clickErr,
			Details:   "failed to evaluate confirm button click",
		}
	}
	resultStr := ""
	if clickResult != nil && clickResult.Result != nil {
		resultStr = clickResult.Result.Value.Str()
	}
	if resultStr != "clicked" {
		op.Error("confirm button click failed", bank.ErrUnknown,
			slog.String("result", resultStr))
		return &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "Logout",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("confirm button click failed: %s", resultStr),
		}
	}
	op.Info("confirm button clicked")

	// Step 4: Wait for redirect, or navigate to login page ourselves.
	// The DELETE completes in <1s; give the Cells redirect 3s to fire.
	redirectCtx, redirectCancel := context.WithTimeout(logoutCtx, 3*time.Second)
	defer redirectCancel()
	if !waitForLogoutRedirect(redirectCtx, s.page) {
		// Headless Chrome doesn't complete the Cells redirect — navigate ourselves.
		// The server-side session is already invalidated by the DELETE above.
		op.Info("completing redirect to login page")
		if navErr := s.page.Navigate(loginURL); navErr != nil {
			op.Error("navigate to login page failed", navErr)
			return &bank.ScraperError{
				Code:      bank.BankBBVA,
				Operation: "Logout",
				Cause:     navErr,
				Details:   "failed to navigate to login page after logout",
			}
		}
	}
	op.Info("redirect complete")

	// Step 5: Clean up — stop hijacker, close page, clear session
	s.stopHijacker()
	_ = s.page.Close()
	s.page = nil
	s.session = nil
	s.debug = nil

	op.Success()
	return nil
}

// waitForLogoutModal polls until the logout confirmation modal becomes visible.
func waitForLogoutModal(ctx context.Context, page *rod.Page) bool {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if browser.DeepQueryExists(page, SelectorLogoutModal) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// waitForLogoutRedirect polls until the page URL no longer contains PortalPath.
func waitForLogoutRedirect(ctx context.Context, page *rod.Page) bool {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := page.Info()
		if err == nil && !strings.Contains(info.URL, PortalPath) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// GetBalance fetches balances for all accounts.
func (s *Scraper) GetBalance(ctx context.Context) ([]bank.Balance, error) {
	op := debug.StartOp(s.logger, "GetBalance")

	if s.page == nil {
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	// Navigate to accounts page with retry (SPA intermittently fails to render).
	if err := navigateToAccountsPage(ctx, s.page, accountsNavStepTimeout, s.logger); err != nil {
		debugCtx, debugCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer debugCancel()
		dp := s.page.Context(debugCtx)

		pageURL, dir := s.debug.Snapshot(dp, "GetBalance", "accounts-timeout")
		diagJSON := s.debug.RunAccountsDiagnostic(dp, "GetBalance", "accounts-timeout-diag", browser.DeepQueryJS, debug.AccountDiagSelectors{
			AccountRow:        SelectorAccountRow,
			AccountCard:       SelectorAccountCard,
			AnnouncementModal: SelectorAnnouncementModal,
		})
		op.Error("accounts page not reachable after retries", bank.ErrUnknown,
			slog.String("url", pageURL), slog.String("debug_dir", dir))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("accounts page not reachable after %d attempts: %v (url=%s, debug=%s, diag=%s)", maxAccountsNavAttempts, err, pageURL, dir, diagJSON),
		}
	}

	// Flatten + parse phase
	flattenCtx, flattenCancel := context.WithTimeout(ctx, s.timeout)
	defer flattenCancel()
	html, _, _, err := browser.FlattenShadowDOM(s.page.Context(flattenCtx))
	if err != nil {
		op.Error("flatten shadow DOM failed", err)
		s.debug.Screenshot(s.page, "GetBalance", "flatten-error")
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("flatten shadow DOM: %v", err),
		}
	}

	balances, err := ParseAccountBalances(html)
	if err != nil {
		s.debug.HTMLString(html, "GetBalance", "parse-error")
		op.Error("parse account balances failed", err, slog.String("debug_dir", s.debug.Dir()))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     err,
			Details:   fmt.Sprintf("parse account balances failed (debug HTML dumped to %s)", s.debug.Dir()),
		}
	}

	op.Success(slog.Int("account_count", len(balances)))
	return balances, nil
}

// GetTransactions fetches transactions for the given account.
func (s *Scraper) GetTransactions(ctx context.Context, accountID string, count int) ([]bank.Transaction, error) {
	op := debug.StartOp(s.logger, "GetTransactions", slog.String("account_id", accountID))

	if count < minTransactionCount {
		count = minTransactionCount
	}
	if count > maxTransactionCount {
		count = maxTransactionCount
	}

	if s.page == nil {
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	// Navigation phase — mimic real user flow:
	// Direct URL navigation leaves the SPA's selectedAccount store empty → "undefined".
	// Instead: accounts page → click "Ir al detalle de cuenta" on the target card.
	// Each step gets its own context to avoid deadline exhaustion across steps.

	// Step 1: Navigate to accounts page with retry (SPA intermittently fails to render)
	if err := navigateToAccountsPage(ctx, s.page, accountsNavStepTimeout, s.logger); err != nil {
		pageURL, dir := s.debug.Snapshot(s.page, "GetTransactions", "accounts-timeout")
		op.Error("accounts page not reachable after retries", bank.ErrUnknown,
			slog.String("url", pageURL), slog.String("debug_dir", dir))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("accounts page not reachable after %d attempts: %v (url=%s, debug=%s)", maxAccountsNavAttempts, err, pageURL, dir),
		}
	}

	// Step 3: Click "Ir al detalle de cuenta" on the card matching this accountID.
	// Each card has a footer link that navigates directly to the account detail page,
	// independent of the SPA's selectedAccount state. This avoids the bug where
	// "Ver todos los movimientos" redirects based on stale SPA state.
	if !waitAndClickAccountDetail(ctx, s.page, accountID, s.timeout) {
		s.debug.Screenshot(s.page, "GetTransactions", "account-not-found")
		op.Error("account card not found", bank.ErrAccountNotFound,
			slog.String("account_id", accountID))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrAccountNotFound,
			Details:   fmt.Sprintf("could not find or click 'Ir al detalle de cuenta' for account %s", accountID),
		}
	}

	// Step 4: Wait for SPA hash navigation to account detail page
	stableCtx, stableCancel := context.WithTimeout(ctx, s.timeout)
	err := s.page.Context(stableCtx).WaitDOMStable(time.Second, 0)
	stableCancel()
	if err != nil {
		s.debug.Screenshot(s.page, "GetTransactions", "dom-unstable")
		op.Error("DOM unstable after account detail click", err)
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("DOM unstable after account detail click: %v", err),
		}
	}

	// Wait for Web Components to finish rendering transaction rows.
	if !waitForTransactionsReady(ctx, s.page, s.timeout) {
		pageURL, dir := s.debug.Snapshot(s.page, "GetTransactions", "table-timeout")
		op.Error("timed out waiting for transactions table", bank.ErrUnknown,
			slog.String("url", pageURL), slog.String("debug_dir", dir))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("timed out waiting for transactions table to render (url=%s, debug=%s)", pageURL, dir),
		}
	}

	// Pagination loop, then extract
	loopCtx, loopCancel := context.WithTimeout(ctx, s.timeout)
	defer loopCancel()
	page := s.page.Context(loopCtx)
	var allTxns []bank.Transaction
	for i := 0; i < maxPaginationClicks; i++ {
		if loopCtx.Err() != nil {
			return nil, &bank.ScraperError{
				Code:      bank.BankBBVA,
				Operation: "GetTransactions",
				Cause:     bank.ErrUnknown,
				Details:   "context cancelled during pagination",
			}
		}

		// Lightweight check - count row elements without flattening
		rowCount := browser.DeepQueryCountAll(page, SelectorTransactionRow)
		op.Info("pagination: checking rows",
			slog.Int("iteration", i),
			slog.Int("rowCount", rowCount),
			slog.Int("target", count))
		if rowCount >= count {
			op.Info("pagination: target reached, stopping")
			break
		}

		if !browser.DeepQueryExists(page, SelectorLoadMoreButton) {
			op.Info("pagination: no 'Ver más' button found, all transactions loaded")
			break // No button — all transactions loaded
		}

		prevCount := rowCount
		// The "Ver más" footer is a web component (bbva-table-footer) with the
		// actual clickable element (bbva-type-link[role="button"]) inside its
		// shadow root. Clicking the outer custom element does nothing — we must
		// deepQuery into its shadow tree for the real button, same pattern as
		// dismissAnnouncementModal.
		clicked, _ := page.Eval(fmt.Sprintf(`() => {
			%s
			const footer = deepQuery(document, '%s');
			if (!footer) return false;
			const link = deepQuery(footer, 'bbva-type-link[role="button"]');
			if (link) { link.click(); return true; }
			// Fallback: try any clickable element inside the footer
			const btn = deepQuery(footer, 'button');
			if (btn) { btn.click(); return true; }
			footer.click();
			return true;
			}`, browser.DeepQueryJS, SelectorLoadMoreButton))
		if !clicked.Value.Bool() {
			op.Info("pagination: click failed, stopping")
			break
		}
		op.Info("pagination: clicked 'Ver más'", slog.Int("iteration", i))
		if err := page.WaitDOMStable(time.Second, 0); err != nil {
			op.Warn("pagination: WaitDOMStable failed after click, continuing to poll")
		}

		// Poll for row count to increase — confirms new rows actually loaded
		rowsLoaded := false
		for j := 0; j < 10; j++ {
			newCount := browser.DeepQueryCountAll(page, SelectorTransactionRow)
			if newCount > prevCount {
				op.Info("pagination: new rows loaded",
					slog.Int("iteration", i),
					slog.Int("prevCount", prevCount),
					slog.Int("newCount", newCount))
				rowsLoaded = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !rowsLoaded {
			op.Info("pagination: no new rows after click, stopping",
				slog.Int("iteration", i),
				slog.Int("rowCount", prevCount))
			break
		}
	}
	loopCancel()

	// Extract the transactions table HTML via deepQuery — clones the subtree
	// and flattens shadow DOM on the clone, leaving the live DOM intact so the
	// SPA framework can still navigate to other routes afterward.
	extractCtx, extractCancel := context.WithTimeout(ctx, s.timeout)
	defer extractCancel()
	html, err := browser.DeepQueryOuterHTML(s.page.Context(extractCtx), SelectorTransactionsTable)
	if err != nil {
		s.debug.Screenshot(s.page, "GetTransactions", "extract-error")
		op.Error("extract transactions table HTML failed", err)
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("extract transactions table HTML: %v", err),
		}
	}
	if html == "" {
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrParsingFailed,
			Details:   "transactions table not found via deepQuery",
		}
	}

	allTxns, err = ParseTransactions(html)
	if err != nil {
		s.debug.HTMLString(html, "GetTransactions", "parse-error")
		op.Error("parse transactions failed", err, slog.String("debug_dir", s.debug.Dir()))
		return nil, &bank.ScraperError{
			Code:      bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     err,
			Details:   fmt.Sprintf("parse transactions failed (debug HTML dumped to %s)", s.debug.Dir()),
		}
	}

	op.Success(slog.Int("transaction_count", len(allTxns)))
	return allTxns, nil
}

// --- PRIVATE DOMAIN LOGIC ---

type loginOutcome int

const (
	loginSuccess loginOutcome = iota
	loginError
	loginTimeout
)

type loginResult struct {
	outcome   loginOutcome
	errorText string
}

// waitForLoginOutcome polls for two conditions after clicking #enviarSenda:
// - URL changes to PortalPath → success (Senda redirected to portal)
// - span#error-message becomes visible with text → failure
//
// In replay mode (s.hijacker != nil), uses a direct Senda API probe
// to bypass the broken iframe postMessage chain.
func (s *Scraper) waitForLoginOutcome(ctx context.Context, page *rod.Page) loginResult {
	// In replay mode, the iframe postMessage chain is broken — use direct API probe
	if s.hijacker != nil {
		return s.probeSendaAPI(ctx, page)
	}

	// Live mode: poll DOM for redirect or error
	waitCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	p := page.Context(waitCtx)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Success: URL changed to portal
		info, err := p.Info()
		if err == nil && strings.Contains(info.URL, PortalPath) {
			return loginResult{outcome: loginSuccess}
		}

		// Error: span#error-message visible with text
		result, err := p.Eval(`() => {
			const el = document.getElementById('error-message');
			if (!el || window.getComputedStyle(el).display === 'none') return '';
			return el.textContent.trim();
		}`)
		if err == nil {
			if text := result.Value.Str(); text != "" {
				return loginResult{outcome: loginError, errorText: text}
			}
		}

		select {
		case <-waitCtx.Done():
			return loginResult{outcome: loginTimeout}
		case <-ticker.C:
		}
	}
}

// probeResponse captures the HTTP status and body from the hijacker.
type probeResponse struct {
	status int
	body   string
}

// probeSendaAPI opens a separate browser tab, sets up its own hijacker, and
// navigates to the grantingTicket endpoint. The hijacker intercepts the
// navigation request and serves the HAR response. This avoids fetch() from
// JS context (which the browser blocks for cross-origin on hijacked pages).
func (s *Scraper) probeSendaAPI(ctx context.Context, _ *rod.Page) loginResult {
	probeCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	probePage, err := s.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return loginResult{outcome: loginTimeout}
	}
	defer func() { _ = probePage.Close() }()

	// Capture grantingTicket response from hijacker
	ch := make(chan probeResponse, 1)
	probeRouter := probePage.HijackRequests()
	probeRouter.MustAdd("*", func(h *rod.Hijack) {
		s.hijacker(h)
		if strings.Contains(h.Request.URL().String(), "grantingTicket") {
			payload := h.Response.Payload()
			ch <- probeResponse{
				status: payload.ResponseCode,
				body:   string(payload.Body),
			}
		}
	})
	go probeRouter.Run()
	defer func() { _ = probeRouter.Stop() }()

	// Navigate triggers the hijacker — response is captured on ch
	if err := probePage.Navigate(SendaAPIURL); err != nil {
		return loginResult{outcome: loginTimeout}
	}

	// Wait for the hijacker to capture the response
	select {
	case resp := <-ch:
		return classifyProbeResponse(resp)
	case <-probeCtx.Done():
		return loginResult{outcome: loginTimeout}
	}
}

// classifyProbeResponse converts a grantingTicket API response into a loginResult.
func classifyProbeResponse(resp probeResponse) loginResult {
	switch resp.status {
	case 200:
		return loginResult{outcome: loginSuccess}

	case 403:
		var apiErr struct {
			ErrorCode string `json:"error-code"`
		}
		if err := json.Unmarshal([]byte(resp.body), &apiErr); err == nil && apiErr.ErrorCode != "" {
			return loginResult{outcome: loginError, errorText: fmt.Sprintf("senda API error-code %s", apiErr.ErrorCode)}
		}
		return loginResult{outcome: loginError, errorText: fmt.Sprintf("senda API 403: %s", resp.body)}

	default:
		return loginResult{outcome: loginError, errorText: fmt.Sprintf("senda API %d: %s", resp.status, resp.body)}
	}
}

// classifySendaError maps Senda error message text to typed errors.
// Handles both UI text (from span#error-message) and API probe format
// ("senda API error-code NNN" or "senda API STATUS: ...").
func classifySendaError(errorText string) error {
	// Probe format: "senda API error-code 160"
	if strings.HasPrefix(errorText, "senda API error-code ") {
		code := strings.TrimPrefix(errorText, "senda API error-code ")
		return classifySendaErrorCode(code)
	}
	// Probe format: "senda API 403: ..." or "senda API probe eval: ..."
	if strings.HasPrefix(errorText, "senda API ") {
		return bank.ErrUnknown
	}
	// UI text matching
	switch {
	case strings.Contains(errorText, "corrijas los datos"):
		return bank.ErrInvalidCredentials
	case strings.Contains(errorText, "bloqueado"):
		return bank.ErrInvalidCredentials
	case strings.Contains(errorText, "límite de intentos"):
		return bank.ErrInvalidCredentials
	case strings.Contains(errorText, "no disponible"):
		return bank.ErrBankUnavailable
	default:
		return bank.ErrUnknown
	}
}

// classifySendaErrorCode maps Senda API error codes to typed errors.
func classifySendaErrorCode(code string) error {
	switch code {
	case "160", "162":
		return bank.ErrInvalidCredentials
	default:
		return bank.ErrUnknown
	}
}

// waitForDashboard polls the page URL for the dashboard route hash.
// The 2026 portal SPA sets this after the "Validando tus credenciales"
// splash transitions to the dashboard.
func (s *Scraper) waitForDashboard(ctx context.Context, page *rod.Page) bool {
	waitCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	p := page.Context(waitCtx)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := p.Info()
		if err == nil && strings.Contains(info.URL, DashboardRoute) {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// navigateTo navigates to the given URL, waits for the page and DOM to
// stabilize, then dismisses the announcement modal if present.
//
// BBVA's portal is a hash-routed SPA. Hash-only URL changes don't trigger
// a full page reload — Chrome treats them as same-page navigation. Since
// FlattenShadowDOM mutates the live DOM (inlining shadow content), subsequent
// hash navigations would operate on a corrupted DOM. A cache-busting query
// param (?_=<timestamp>) before the hash fragment forces Chrome to treat
// each navigation as a new URL → full page load guaranteed. The SPA ignores
// query params; the hash router handles #!/route normally.
func navigateTo(ctx context.Context, page *rod.Page, url string) error {
	p := page.Context(ctx)
	if err := p.Navigate(cacheBust(url)); err != nil {
		return fmt.Errorf("navigate to %s: %w", url, err)
	}
	if err := p.WaitLoad(); err != nil {
		return fmt.Errorf("wait load %s: %w", url, err)
	}
	if err := p.WaitDOMStable(time.Second, 0); err != nil {
		return fmt.Errorf("wait DOM stable %s: %w", url, err)
	}
	dismissAnnouncementModal(ctx, page)
	return nil
}

// navigateToAccountsPage navigates to the accounts page and waits for Web
// Components to render account data. Retries up to maxAccountsNavAttempts
// times because the SPA framework intermittently fails to render route content.
// Each retry triggers a fresh page load via cache-busted URL.
func navigateToAccountsPage(ctx context.Context, page *rod.Page, timeout time.Duration, logger *slog.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= maxAccountsNavAttempts; attempt++ {
		navCtx, navCancel := context.WithTimeout(ctx, timeout)
		err := navigateTo(navCtx, page, accountsURL)
		navCancel()
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: navigate: %w", attempt, err)
			if attempt < maxAccountsNavAttempts {
				logger.Warn("accounts page navigation failed, retrying",
					slog.Int("attempt", attempt),
					slog.Int("max_attempts", maxAccountsNavAttempts),
					slog.Any("error", err))
			}
			continue
		}

		if waitForAccountsReady(ctx, page, timeout) {
			if attempt > 1 {
				logger.Info("accounts page loaded on retry",
					slog.Int("attempt", attempt))
			}
			return nil
		}

		lastErr = fmt.Errorf("attempt %d: accounts did not render within %s", attempt, timeout)
		if attempt < maxAccountsNavAttempts {
			logger.Warn("accounts page did not render, retrying",
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", maxAccountsNavAttempts))
		}
	}
	return lastErr
}

// cacheBust inserts ?_=<nanosecond-timestamp> before the hash fragment.
func cacheBust(rawURL string) string {
	q := "?_=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if i := strings.Index(rawURL, "#"); i >= 0 {
		return rawURL[:i] + q + rawURL[i:]
	}
	return rawURL + q
}

// dismissAnnouncementModal dismisses the news/announcement popup if present.
// Non-blocking: if the modal isn't found or click fails, navigation continues.
//
// The modal's buttons live deep inside nested shadow roots (e.g.,
// modal → shadowRoot → bbva-button → shadowRoot → <button>). Neither
// standard querySelector nor Element.matches() with descendant selectors
// can cross shadow boundaries. We use deepQuery rooted at the modal element
// to walk its entire shadow tree and find any clickable button.
func dismissAnnouncementModal(ctx context.Context, page *rod.Page) {
	modalCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	p := page.Context(modalCtx)

	// Find the modal, then search its shadow tree for a button to click.
	// deepQuery(modal, 'button') walks shadow+light DOM from the modal down.
	_, _ = p.Eval(fmt.Sprintf(`() => {
		%s
		const modal = deepQuery(document, '%s');
		if (!modal) return;
		const btn = deepQuery(modal, 'button');
		if (btn) btn.click();
	}`, browser.DeepQueryJS, SelectorAnnouncementModal))
}

// clickAccountDetail finds and clicks the "Ir al detalle de cuenta" footer link
// on the card matching the given accountID. Each card has a direct link that
// navigates to the account detail page without depending on SPA selection state.
func clickAccountDetail(ctx context.Context, page *rod.Page, accountID string) bool {
	js := fmt.Sprintf(`() => {
		%s
		const card = deepQuery(document, '%s#%s');
		if (!card) return false;
		const link = deepQuery(card, '%s');
		if (!link) return false;
		link.click();
		return true;
	}`, browser.DeepQueryJS, SelectorAccountCard, accountID, SelectorCardFooterLink)

	result, err := page.Context(ctx).Eval(js)
	if err != nil {
		return false
	}
	return result.Value.Bool()
}

// waitAndClickAccountDetail polls at 200ms intervals until clickAccountDetail
// succeeds. The card's footer link only becomes clickable once the accounts
// page has fully rendered.
func waitAndClickAccountDetail(ctx context.Context, page *rod.Page, accountID string, timeout time.Duration) bool {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if clickAccountDetail(waitCtx, page, accountID) {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// waitForTransactionsReady polls until the transactions table has rendered
// content or reached a terminal state. Web Components render asynchronously —
// WaitDOMStable fires before shadow content is fully populated.
//
// Terminal states: data rows rendered, "noresults", or any non-empty state
// attribute (including error states like "La información no está disponible").
func waitForTransactionsReady(ctx context.Context, page *rod.Page, timeout time.Duration) bool {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p := page.Context(waitCtx)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Any non-empty state attr means the table has reached a terminal state
		// (e.g., "noresults", "error", or similar). Let the parser handle it.
		if state := browser.DeepQueryAttr(p, SelectorTransactionsTable, "state"); state != "" {
			return true
		}
		// First row has a populated date attr — data has rendered
		if browser.DeepQueryAttr(p, SelectorTxOperationDate, "date") != "" {
			return true
		}
		// Table exists (even without state attr) — the page has loaded enough
		if browser.DeepQueryExists(p, SelectorTransactionsTable) {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// waitForAccountsReady polls until the accounts page has rendered either
// list view rows or tile view cards with data.
func waitForAccountsReady(ctx context.Context, page *rod.Page, timeout time.Duration) bool {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p := page.Context(waitCtx)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		// List view: table with at least one data row
		if browser.DeepQueryExists(p, SelectorAccountRow) {
			return true
		}
		// Tile view: at least one card with a product-amount (skip allContracts card)
		if browser.DeepQueryAttr(p, SelectorAccountCard+"[product-amount]", "product-amount") != "" {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func fillLoginForm(page *rod.Page, creds credentials, typeFn func(*rod.Element, string) error) error {
	companyInput, err := page.Element(SelectorCompanyInput)
	if err != nil {
		return fmt.Errorf("company input not found: %w", err)
	}
	if err := typeFn(companyInput, creds.companyCode); err != nil {
		return fmt.Errorf("failed to type company code: %w", err)
	}

	userInput, err := page.Element(SelectorUserInput)
	if err != nil {
		return fmt.Errorf("user input not found: %w", err)
	}
	if err := typeFn(userInput, creds.userCode); err != nil {
		return fmt.Errorf("failed to type user code: %w", err)
	}

	passwordInput, err := page.Element(SelectorPasswordInput)
	if err != nil {
		return fmt.Errorf("password input not found: %w", err)
	}
	if err := typeFn(passwordInput, creds.password); err != nil {
		return fmt.Errorf("failed to type password: %w", err)
	}
	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("bbva-%d", time.Now().UnixNano())
}

// debugDir returns the base directory for debug artifacts.
func debugDir() string {
	return filepath.Join(os.TempDir(), debugBaseDir)
}
