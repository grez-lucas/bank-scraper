// Package bbva defines the scraper and parsing logic to process the BBVA
// portal.
package bbva

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	maxPaginationClicks = 20 // Safety limit for "Ver más" pagination loop

	defaultTimeout = 30 * time.Second

	bbvaSessionTimeout = 10 * time.Minute
)

type BBVAScraper struct {
	browser  *rod.Browser
	page     *rod.Page          // Authenticated page, kept alive between operations
	router   *rod.HijackRouter  // Request hijacker, kept alive with the page
	session  *bank.Session
	timeout  time.Duration
	hijacker func(*rod.Hijack) // Optional hijacker for replay testing
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

// WithHijacker sets a custom hijacker middleware for request interception.
// This is used for replay testing to serve recorded responses instead of
// making real network requests.
func WithHijacker(middleware func(*rod.Hijack)) Option {
	return func(s *BBVAScraper) {
		s.hijacker = middleware
	}
}

func NewBBVAScraper(opts ...Option) (*BBVAScraper, error) {
	// Launch w/ stealth mode
	url, err := launcher.New().Headless(true).Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	s := &BBVAScraper{
		browser: browser,
		timeout: defaultTimeout,
	}

	for _, opt := range opts {
		opt(s)
	}

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

	// Set up request hijacking BEFORE applying timeout.
	// The router's event context derives from the page's context at creation time.
	// If we set it up after Timeout(), CancelTimeout() would kill the router's context.
	router := page.HijackRequests()

	if s.hijacker != nil {
		router.MustAdd("*", s.hijacker)
	} else {
		router.MustAdd("*", func(ctx *rod.Hijack) {
			ctx.LoadResponse(http.DefaultClient, true)
		})
	}

	go router.Run()
	s.router = router

	page = page.Timeout(s.timeout)

	// Wait for the login form
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("login page load failed: %w", err)
	}

	// 1. Fill credentials (form is on main page, not in iframe)
	if err := fillLoginForm(page, creds); err != nil {
		return nil, err
	}

	// 2. Click login (#enviarSenda → Senda flow via postMessage to iframe)
	loginBtn, err := page.Element(SelectorLoginButton)
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

	// 3. Wait for Senda outcome: portal redirect (success) or error span (failure)
	// Cancel the page-level timeout so waitForLoginOutcome can use its own deadline.
	// The page stays timeout-free; each method (GetBalance, etc.) sets a fresh timeout.
	page = page.CancelTimeout()
	result := s.waitForLoginOutcome(page)
	switch result.outcome {
	case loginSuccess:
		if s.hijacker == nil {
			// Live mode: wait for the SPA to set the dashboard route hash
			if !s.waitForDashboard(page) {
				return nil, &bank.ScraperError{
					BankCode:  bank.BankBBVA,
					Operation: "Login",
					Cause:     bank.ErrUnknown,
					Details:   "login completed but dashboard did not load",
				}
			}
			dismissAnnouncementModal(page)
		}

	case loginError:
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     classifySendaError(result.errorText),
			Details:   result.errorText,
		}

	case loginTimeout:
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrUnknown,
			Details:   "login timed out waiting for redirect or error",
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

	page := s.page.Timeout(s.timeout)
	if err := navigateTo(page, accountsURL); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("navigate to accounts: %v", err),
		}
	}

	html, _, _, err := browser.FlattenShadowDOM(page)
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
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetBalance",
			Cause:     err,
			Details:   "parse account balances failed",
		}
	}

	return balances, nil
}

// transactionsURL builds the URL for a specific account's movements page.
func transactionsURL(accountID string) string {
	return portalURL + "#!/bbva-btge-accounts-solution/account/" + accountID + "/movements"
}

func (s *BBVAScraper) GetTransactions(ctx context.Context, accountID string) ([]bank.Transaction, error) {
	if s.page == nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrSessionExpired,
			Details:   "no active session — call Login first",
		}
	}

	page := s.page.Timeout(s.timeout)
	if err := navigateTo(page, transactionsURL(accountID)); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "GetTransactions",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("navigate to transactions: %v", err),
		}
	}

	var allTxns []bank.Transaction
	for i := 0; ; i++ {
		html, _, _, err := browser.FlattenShadowDOM(page)
		if err != nil {
			return nil, &bank.ScraperError{
				BankCode:  bank.BankBBVA,
				Operation: "GetTransactions",
				Cause:     bank.ErrUnknown,
				Details:   fmt.Sprintf("flatten shadow DOM: %v", err),
			}
		}

		txns, err := ParseTransactions(html)
		if err != nil {
			return nil, &bank.ScraperError{
				BankCode:  bank.BankBBVA,
				Operation: "GetTransactions",
				Cause:     err,
				Details:   "parse transactions failed",
			}
		}

		allTxns = txns

		if !HasMoreTransactions(html) || i >= maxPaginationClicks {
			break
		}

		// Click "Ver más" and wait for DOM to update with new rows
		loadMoreBtn, err := page.Element(SelectorLoadMoreButton)
		if err != nil {
			break // Button not found in live DOM — stop paginating
		}
		if err := loadMoreBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
			break
		}
		page.MustWaitDOMStable()
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
func (s *BBVAScraper) waitForLoginOutcome(page *rod.Page) loginResult {
	// In replay mode, the iframe postMessage chain is broken — use direct API probe
	if s.hijacker != nil {
		return s.probeSendaAPI(page)
	}

	// Live mode: poll DOM for redirect or error
	deadline := time.Now().Add(s.timeout)
	for time.Now().Before(deadline) {
		// Success: URL changed to portal
		info, err := page.Timeout(2 * time.Second).Info()
		if err == nil && strings.Contains(info.URL, PortalPath) {
			return loginResult{outcome: loginSuccess}
		}

		// Error: span#error-message visible with text
		result, err := page.Timeout(2 * time.Second).Eval(`() => {
			const el = document.getElementById('error-message');
			if (!el || window.getComputedStyle(el).display === 'none') return '';
			return el.textContent.trim();
		}`)
		if err == nil {
			if text := result.Value.Str(); text != "" {
				return loginResult{outcome: loginError, errorText: text}
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
	return loginResult{outcome: loginTimeout}
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
func (s *BBVAScraper) probeSendaAPI(page *rod.Page) loginResult {
	probePage, err := s.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return loginResult{outcome: loginTimeout}
	}
	defer func() { _ = probePage.Close() }()

	// Capture grantingTicket response from hijacker
	ch := make(chan probeResponse, 1)
	probeRouter := probePage.HijackRequests()
	probeRouter.MustAdd("*", func(ctx *rod.Hijack) {
		s.hijacker(ctx)
		if strings.Contains(ctx.Request.URL().String(), "grantingTicket") {
			payload := ctx.Response.Payload()
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
	case <-time.After(s.timeout):
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
func (s *BBVAScraper) waitForDashboard(page *rod.Page) bool {
	deadline := time.Now().Add(s.timeout)
	for time.Now().Before(deadline) {
		info, err := page.Info()
		if err == nil && strings.Contains(info.URL, DashboardRoute) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// navigateTo navigates to the given URL, waits for the page and DOM to
// stabilize, then dismisses the announcement modal if present.
// Use this for all post-login page navigation.
func navigateTo(page *rod.Page, url string) error {
	if err := page.Navigate(url); err != nil {
		return fmt.Errorf("navigate to %s: %w", url, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("wait load %s: %w", url, err)
	}
	page.MustWaitDOMStable()
	dismissAnnouncementModal(page)
	return nil
}

// dismissAnnouncementModal dismisses the news/announcement popup if present.
// Non-blocking: if the modal isn't found or click fails, navigation continues.
func dismissAnnouncementModal(page *rod.Page) {
	el, err := page.Timeout(3 * time.Second).Element(SelectorAnnouncementCloseBtn)
	if err != nil {
		return // Modal not present
	}
	visible, err := el.Visible()
	if err != nil || !visible {
		return
	}
	_ = el.Click(proto.InputMouseButtonLeft, 1)
	time.Sleep(500 * time.Millisecond)
}

func fillLoginForm(page *rod.Page, creds Credentials) error {
	companyInput, err := page.Element(SelectorCompanyInput)
	if err != nil {
		return fmt.Errorf("company input not found: %w", err)
	}
	if err := browser.TypeFast(companyInput, creds.CompanyCode); err != nil {
		return fmt.Errorf("failed to type company code: %w", err)
	}

	userInput, err := page.Element(SelectorUserInput)
	if err != nil {
		return fmt.Errorf("user input not found: %w", err)
	}
	if err := browser.TypeFast(userInput, creds.UserCode); err != nil {
		return fmt.Errorf("failed to type user code: %w", err)
	}

	passwordInput, err := page.Element(SelectorPasswordInput)
	if err != nil {
		return fmt.Errorf("password input not found: %w", err)
	}
	if err := browser.TypeFast(passwordInput, creds.Password); err != nil {
		return fmt.Errorf("failed to type password: %w", err)
	}
	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("bbva-%d", time.Now().UnixNano())
}
