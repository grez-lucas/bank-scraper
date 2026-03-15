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
)

const (
	baseURL     = "https://www.bbvanetcash.pe"
	loginURL    = baseURL + "/DFAUTH85/mult/KDPOSolicitarCredenciales_es.html"
	portalURL   = baseURL + "/nextgenempresas/portal/index.html"
	accountsURL = baseURL + "/nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution"

	maxPaginationClicks = 10 // Safety limit for "Ver más" pagination loop

	defaultTimeout = 30 * time.Second

	bbvaSessionTimeout = 10 * time.Minute

	minTransactionCount = 50
	maxTransactionCount = 250
)

type BBVAScraper struct {
	browser  *rod.Browser
	page     *rod.Page         // Authenticated page, kept alive between operations
	router   *rod.HijackRouter // Request hijacker, kept alive with the page
	session  *bank.Session
	timeout  time.Duration
	headless bool              // Whether to launch browser in headless mode
	hijacker func(*rod.Hijack) // Optional hijacker for replay testing
	logger   *slog.Logger
}

type Credentials struct {
	CompanyCode string
	UserCode    string
	Password    string
}

// Option pattern for configuration
type Option func(*BBVAScraper)

func WithTimeout(d time.Duration) Option {
	return func(s *BBVAScraper) {
		s.timeout = d
	}
}

// WithHeadless controls whether the browser launches in headless mode.
// Default is true. Set to false for visual debugging of live sessions.
func WithHeadless(headless bool) Option {
	return func(s *BBVAScraper) {
		s.headless = headless
	}
}

// WithHijacker sets a custom hijacker middleware for request interception.
// This is used for replay testing to serve recorded responses instead of
// making real network requests.
func WithHijacker(middleware func(*rod.Hijack)) Option {
	return func(s *BBVAScraper) {
		s.hijacker = middleware
	}
}

func WithLogger(l *slog.Logger) Option {
	return func(s *BBVAScraper) {
		s.logger = l
	}
}

func NewBBVAScraper(opts ...Option) (*BBVAScraper, error) {
	s := &BBVAScraper{
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

func (s *BBVAScraper) Login(ctx context.Context, creds Credentials) (*bank.Session, error) {
	// Close previous page if re-logging in
	if s.page != nil {
		s.stopHijacker()
		_ = s.page.Close()
		s.page = nil
		s.session = nil
	}

	page, err := s.browser.Page(proto.TargetCreateTarget{URL: loginURL})
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
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
			h.LoadResponse(http.DefaultClient, true)
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
		return nil, fmt.Errorf("login page load failed: %w", err)
	}

	// 1. Fill credentials (form is on main page, not in iframe)
	// Use human-like typing in live mode to avoid bot detection
	typeFn := browser.TypeFast
	if s.hijacker == nil {
		typeFn = browser.TypeHuman
	}
	if err := fillLoginForm(p, creds, typeFn); err != nil {
		return nil, err
	}

	// Small random delay before clicking to appear more human-like
	if s.hijacker == nil {
		time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
	}

	// 2. Click login (#enviarSenda → Senda flow via postMessage to iframe)
	loginBtn, err := p.Element(SelectorLoginButton)
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("login button not found: %v", err),
		}
	}
	if err := loginBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
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
				return nil, &bank.ScraperError{
					BankCode:  bank.BankBBVA,
					Operation: "Login",
					Cause:     bank.ErrUnknown,
					Details:   "login completed but dashboard did not load",
				}
			}
			dismissAnnouncementModal(ctx, page)
		}

	case loginError:
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     classifySendaError(result.errorText),
			Details:   result.errorText,
		}

	case loginTimeout:
		// Capture screenshot and URL for debugging
		debugDir := filepath.Join(os.TempDir(), "bbva-debug")
		_ = os.MkdirAll(debugDir, 0o755)
		screenshotPath := filepath.Join(debugDir, "login-timeout.png")
		screenshot, _ := page.Screenshot(true, nil)
		if len(screenshot) > 0 {
			_ = os.WriteFile(screenshotPath, screenshot, 0o644)
		}
		info, _ := page.Info()
		pageURL := ""
		if info != nil {
			pageURL = info.URL
		}
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("login timed out waiting for redirect or error (url=%s, screenshot=%s)", pageURL, screenshotPath),
		}
	}

	// Store session and page for subsequent operations
	session := &bank.Session{
		ID:        generateSessionID(),
		BankCode:  bank.BankBBVA,
		ExpiresAt: time.Now().Add(bbvaSessionTimeout),
	}
	s.session = session
	s.page = page
	success = true

	return session, nil
}

func (s *BBVAScraper) Close() error {
	s.stopHijacker()
	if s.page != nil {
		_ = s.page.Close()
		s.page = nil
	}
	s.session = nil
	if s.browser != nil {
		return s.browser.Close()
	}
	return nil
}

func (s *BBVAScraper) stopHijacker() {
	if s.router != nil {
		s.router.Stop()
		s.router = nil
	}
}

func (s *BBVAScraper) GetBalance(ctx context.Context) ([]bank.Balance, error) {
	if s.page == nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	// Navigation phase
	navCtx, navCancel := context.WithTimeout(ctx, s.timeout)
	defer navCancel()
	if err := navigateTo(navCtx, s.page, accountsURL); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("navigate to accounts: %v", err),
		}
	}

	// Wait for Web Components to finish rendering account data.
	// Uses its own derived context — independent of the navigation deadline.
	if !waitForAccountsReady(ctx, s.page, s.timeout) {
		debugCtx, debugCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer debugCancel()
		dp := s.page.Context(debugCtx)

		debugDir := filepath.Join(os.TempDir(), "bbva-debug")
		_ = os.MkdirAll(debugDir, 0o755)

		screenshot, err := dp.Screenshot(true, nil)
		if err == nil && len(screenshot) > 0 {
			_ = os.WriteFile(filepath.Join(debugDir, "accounts-timeout.png"), screenshot, 0o644)
		}
		html, _ := dp.HTML()
		if len(html) > 0 {
			_ = os.WriteFile(filepath.Join(debugDir, "accounts-timeout.html"), []byte(html), 0o644)
		}

		// Diagnostic: trace DOM structure to find where deepQuery loses the path.
		// Uses the same walk as flattenShadowDOMJS (which works) to catalog all
		// shadow hosts and their children, plus tries deepQuery.
		diagJS := fmt.Sprintf(`() => {
			%s
			const d = {};

			// 1. deepQuery results
			d.dqAccountRow = deepQuery(document, '%s') !== null;
			d.dqAnyTR = deepQuery(document, 'tr') !== null;
			d.dqAnyCard = deepQuery(document, '%s') !== null;
			d.dqModal = deepQuery(document, '%s') !== null;

			// 2. Walk using flattenShadowDOM's approach (childNodes + shadowRoot)
			// and collect all tag names found, plus shadow host paths.
			const tags = {};
			const shadowHosts = [];
			function walk2(node, path, depth) {
				if (depth > 50) return;
				const tag = node.tagName ? node.tagName.toLowerCase() : '#text';
				tags[tag] = (tags[tag] || 0) + 1;

				if (node.shadowRoot) {
					shadowHosts.push(path + '/' + tag + '#shadow');
					// Light DOM children first (like flattenShadowDOM)
					for (const c of Array.from(node.childNodes)) {
						if (c.nodeType === 1) walk2(c, path + '/' + tag + '.light', depth+1);
					}
					// Shadow DOM children
					for (const c of Array.from(node.shadowRoot.childNodes)) {
						if (c.nodeType === 1) walk2(c, path + '/' + tag + '.shadow', depth+1);
					}
				} else {
					for (const c of Array.from(node.childNodes)) {
						if (c.nodeType === 1) walk2(c, path + '/' + tag, depth+1);
					}
				}
			}
			walk2(document.documentElement, '', 0);
			d.shadowHostCount = shadowHosts.length;
			d.tagCounts = tags;

			// 3. Check specific elements we expect
			d.hasTR = (tags['tr'] || 0) > 0;
			d.hasTable = (tags['table'] || 0) > 0;
			d.hasCard = (tags['bbva-btge-card-product-select'] || 0) > 0;
			d.hasAccountsTable = (tags['bbva-btge-accounts-solution-table'] || 0) > 0;

			// 4. Dump first 20 shadow hosts to see the nesting
			d.shadowHostPaths = shadowHosts.slice(0, 20);

			return JSON.stringify(d);
		}`, browser.DeepQueryJS, SelectorAccountRow, SelectorAccountCard, SelectorAnnouncementModal)
		diagResult, diagErr := dp.Eval(diagJS)
		diagJSON := ""
		if diagErr == nil {
			diagJSON = diagResult.Value.Str()
		} else {
			diagJSON = fmt.Sprintf("eval error: %v", diagErr)
		}
		_ = os.WriteFile(filepath.Join(debugDir, "accounts-diag.json"), []byte(diagJSON), 0o644)

		info, _ := dp.Info()
		pageURL := ""
		if info != nil {
			pageURL = info.URL
		}
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("timed out waiting for accounts to render (url=%s, debug=%s, diag=%s)", pageURL, debugDir, diagJSON),
		}
	}

	// Flatten + parse phase
	flattenCtx, flattenCancel := context.WithTimeout(ctx, s.timeout)
	defer flattenCancel()
	html, _, _, err := browser.FlattenShadowDOM(s.page.Context(flattenCtx))
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("flatten shadow DOM: %v", err),
		}
	}

	balances, err := ParseAccountBalances(html)
	if err != nil {
		debugDir := filepath.Join(os.TempDir(), "bbva-debug")
		_ = os.MkdirAll(debugDir, 0o755)
		debugPath := filepath.Join(debugDir, "balances-debug.html")
		_ = os.WriteFile(debugPath, []byte(html), 0o644)
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     err,
			Details:   fmt.Sprintf("parse account balances failed (debug HTML dumped to %s)", debugPath),
		}
	}

	return balances, nil
}

// transactionsURL builds the URL for a specific account's movements page.
func transactionsURL(accountID string) string {
	return portalURL + "#!/bbva-btge-accounts-solution/account/" + accountID + "/movements"
}

func (s *BBVAScraper) GetTransactions(ctx context.Context, accountID string, count int) ([]bank.Transaction, error) {
	if count < minTransactionCount {
		count = minTransactionCount
	}
	if count > maxTransactionCount {
		count = maxTransactionCount
	}

	if s.page == nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	// Navigation phase — mimic real user flow:
	// Direct URL navigation leaves the SPA's selectedAccount store empty → "undefined".
	// Instead: accounts page → click account card → click "Ver todos los movimientos".
	navCtx, navCancel := context.WithTimeout(ctx, s.timeout)
	defer navCancel()

	// Step 1: Navigate to accounts page (reload clears prior FlattenShadowDOM mutations)
	if err := navigateTo(navCtx, s.page, accountsURL); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("navigate to accounts: %v", err),
		}
	}

	// Step 2: Wait for accounts to render, then click the target card
	if !waitForAccountsReady(ctx, s.page, s.timeout) {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   "timed out waiting for accounts page to render",
		}
	}

	// Click the card matching this accountID (sets SPA selectedAccount state)
	cardSelector := fmt.Sprintf(`%s#%s`, SelectorAccountCard, accountID)
	if !browser.DeepQueryClick(s.page.Context(navCtx), cardSelector) {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrAccountNotFound,
			Details:   fmt.Sprintf("account card not found: %s", cardSelector),
		}
	}

	// Step 3: Poll for "Ver todos los movimientos" link and click it.
	// After the card click, the "Últimos movimientos" section renders
	// asynchronously. Instead of a blanket WaitDOMStable, we poll until
	// clickViewAllMovements succeeds (the link appears).
	if !waitAndClickViewAll(navCtx, s.page) {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   "could not find or click 'Ver todos los movimientos' link",
		}
	}

	// Wait for SPA hash navigation to transactions page
	if err := s.page.Context(navCtx).WaitDOMStable(time.Second, 0); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("DOM unstable after view-all click: %v", err),
		}
	}
	navCancel() // Navigation phase complete

	// Wait for Web Components to finish rendering transaction rows.
	if !waitForTransactionsReady(ctx, s.page, s.timeout) {
		debugDir := filepath.Join(os.TempDir(), "bbva-debug")
		_ = os.MkdirAll(debugDir, 0o755)
		screenshotPath := filepath.Join(debugDir, "transactions-timeout.png")
		screenshot, _ := s.page.Screenshot(true, nil)
		if len(screenshot) > 0 {
			_ = os.WriteFile(screenshotPath, screenshot, 0o644)
		}
		info, _ := s.page.Info()
		pageURL := ""
		if info != nil {
			pageURL = info.URL
		}
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("timed out waiting for transactions table to render (url=%s, screenshot=%s)", pageURL, screenshotPath),
		}
	}

	// TODO: The last thing we do should be flattening. First pagination loop, then flatten.
	// Flatten + parse + pagination loop
	loopCtx, loopCancel := context.WithTimeout(ctx, s.timeout)
	defer loopCancel()
	page := s.page.Context(loopCtx)
	var allTxns []bank.Transaction
	for i := 0; i < maxPaginationClicks; i++ {
		if loopCtx.Err() != nil {
			return nil, &bank.ScraperError{
				BankCode:  bank.BankBBVA,
				Operation: "GetTransactions",
				Cause:     bank.ErrUnknown,
				Details:   "context cancelled during pagination",
			}
		}

		// Lightweight check - count row elements without flattening
		rowCount := browser.DeepQueryCountAll(page, SelectorTransactionRow)
		s.logger.Info("pagination: checking rows",
			slog.Int("iteration", i),
			slog.Int("rowCount", rowCount),
			slog.Int("target", count))
		if rowCount >= count {
			s.logger.Info("pagination: target reached, stopping")
			break
		}

		if !browser.DeepQueryExists(page, SelectorLoadMoreButton) {
			s.logger.Info("pagination: no 'Ver más' button found, all transactions loaded")
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
			s.logger.Info("pagination: click failed, stopping")
			break
		}
		s.logger.Info("pagination: clicked 'Ver más'", slog.Int("iteration", i))
		if err := page.WaitDOMStable(time.Second, 0); err != nil {
			s.logger.Warn("pagination: WaitDOMStable failed after click, continuing to poll")
		}

		// Poll for row count to increase — confirms new rows actually loaded
		rowsLoaded := false
		for j := 0; j < 10; j++ {
			newCount := browser.DeepQueryCountAll(page, SelectorTransactionRow)
			if newCount > prevCount {
				s.logger.Info("pagination: new rows loaded",
					slog.Int("iteration", i),
					slog.Int("prevCount", prevCount),
					slog.Int("newCount", newCount))
				rowsLoaded = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !rowsLoaded {
			s.logger.Info("pagination: no new rows after click, stopping",
				slog.Int("iteration", i),
				slog.Int("rowCount", prevCount))
			break
		}
	}
	loopCancel()

	// Extract just the transactions table HTML via deepQuery — much faster
	// than flattening the entire page DOM. The parser only reads attributes
	// on custom elements (light DOM), so shadow DOM flattening isn't needed.
	extractCtx, extractCancel := context.WithTimeout(ctx, s.timeout)
	defer extractCancel()
	html, err := browser.DeepQueryHTML(s.page.Context(extractCtx), SelectorTransactionsTable)
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("extract transactions table HTML: %v", err),
		}
	}
	if html == "" {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrParsingFailed,
			Details:   "transactions table not found via deepQuery",
		}
	}

	allTxns, err = ParseTransactions(html)
	if err != nil {
		debugDir := filepath.Join(os.TempDir(), "bbva-debug")
		_ = os.MkdirAll(debugDir, 0o755)
		debugPath := filepath.Join(debugDir, "transactions-debug.html")
		_ = os.WriteFile(debugPath, []byte(html), 0o644)
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     err,
			Details:   fmt.Sprintf("parse transactions failed (debug HTML dumped to %s)", debugPath),
		}
	}

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
func (s *BBVAScraper) waitForLoginOutcome(ctx context.Context, page *rod.Page) loginResult {
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
func (s *BBVAScraper) probeSendaAPI(ctx context.Context, page *rod.Page) loginResult {
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
	defer probeRouter.Stop()

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
	switch {
	case resp.status == 200:
		return loginResult{outcome: loginSuccess}

	case resp.status == 403:
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
func (s *BBVAScraper) waitForDashboard(ctx context.Context, page *rod.Page) bool {
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

// clickViewAllMovements finds and clicks the "Ver todos los movimientos" link
// on the accounts page. The link is inside shadow DOM, so we use deepQuery to
// find the container, then querySelector within it for the link element.
func clickViewAllMovements(ctx context.Context, page *rod.Page) bool {
	js := fmt.Sprintf(`() => {
		%s
		const container = deepQuery(document, '%s');
		if (!container) return false;
		const link = container.querySelector('bbva-type-link');
		if (!link) return false;
		link.click();
		return true;
	}`, browser.DeepQueryJS, SelectorViewAllMovements)

	result, err := page.Context(ctx).Eval(js)
	if err != nil {
		return false
	}
	return result.Value.Bool()
}

// waitAndClickViewAll polls at 200ms intervals until clickViewAllMovements
// succeeds. This replaces the blanket WaitDOMStable after clicking an account
// card — the "Ver todos" link only appears once the "Últimos movimientos"
// section finishes rendering.
func waitAndClickViewAll(ctx context.Context, page *rod.Page) bool {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if clickViewAllMovements(ctx, page) {
			return true
		}
		select {
		case <-ctx.Done():
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

func fillLoginForm(page *rod.Page, creds Credentials, typeFn func(*rod.Element, string) error) error {
	companyInput, err := page.Element(SelectorCompanyInput)
	if err != nil {
		return fmt.Errorf("company input not found: %w", err)
	}
	if err := typeFn(companyInput, creds.CompanyCode); err != nil {
		return fmt.Errorf("failed to type company code: %w", err)
	}

	userInput, err := page.Element(SelectorUserInput)
	if err != nil {
		return fmt.Errorf("user input not found: %w", err)
	}
	if err := typeFn(userInput, creds.UserCode); err != nil {
		return fmt.Errorf("failed to type user code: %w", err)
	}

	passwordInput, err := page.Element(SelectorPasswordInput)
	if err != nil {
		return fmt.Errorf("password input not found: %w", err)
	}
	if err := typeFn(passwordInput, creds.Password); err != nil {
		return fmt.Errorf("failed to type password: %w", err)
	}
	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("bbva-%d", time.Now().UnixNano())
}
