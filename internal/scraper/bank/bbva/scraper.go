// Package bbva defines the scraper and parsing logic to process the BBVA
// portal.
package bbva

import (
	"context"
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
	baseURL  = "https://www.bbvanetcash.pe"
	loginURL = baseURL + "/DFAUTH85/mult/KDPOSolicitarCredenciales_es.html"

	// dfServletPath is the login form submission endpoint - we capture its
	// status code to detect bot detection (403) or other server errors.
	dfServletPath = "/DFAUTH85/slod_pe_web/DFServlet"

	defaultTimeout = 30 * time.Second

	bbvaSessionTimeout = 10 * time.Minute
)

type BBVAScraper struct {
	browser  *rod.Browser
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
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: loginURL})
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   err.Error(),
		}
	}
	defer page.Close()

	page = page.Timeout(s.timeout)

	// Set up request hijacking
	var statusCode int
	router := page.HijackRequests()

	if s.hijacker != nil {
		// Use custom hijacker (for replay testing) and capture status code
		router.MustAdd("*", func(ctx *rod.Hijack) {
			s.hijacker(ctx)
			// Only capture status code for the login form submission (DFServlet)
			// Ignore analytics/tracking requests that would overwrite with 200
			if ctx.Response != nil && strings.Contains(ctx.Request.URL().Path, dfServletPath) {
				statusCode = ctx.Response.Payload().ResponseCode
			}
		})
	} else {
		// Default: capture HTTP status code from real requests
		router.MustAdd("*", func(ctx *rod.Hijack) {
			ctx.LoadResponse(http.DefaultClient, true)
			// Only capture status code for the login form submission (DFServlet)
			// Ignore analytics/tracking requests that would overwrite with 200
			if ctx.Response != nil && strings.Contains(ctx.Request.URL().Path, dfServletPath) {
				statusCode = ctx.Response.Payload().ResponseCode
			}
		})
	}

	go router.Run()
	defer router.Stop()

	// Wait for the login form
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("login page load failed: %w", err)
	}

	// 1. Fill credentials (form is on main page, not in iframe)
	if err := fillLoginForm(page, creds); err != nil {
		return nil, err
	}

	// 2. Click login
	statusCode = 0 // Reset before submission
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

	// Wait for navigation
	if err := page.WaitLoad(); err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrBankUnavailable,
			Details:   fmt.Sprintf("post-login load failed: %v", err),
		}
	}

	html, err := page.HTML()
	if err != nil {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrUnknown,
			Details:   fmt.Sprintf("failed to get page HTML: %v", err),
		}
	}

	// Check for errors
	if loginErr := DetectLoginError(html, statusCode); loginErr != nil {
		cause := bank.ErrInvalidCredentials
		if info, ok := loginErr.(*LoginErrorInfo); ok && info.HTTPStatus == http.StatusForbidden {
			cause = bank.ErrBotDetection
		}
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     cause,
			Details:   loginErr.Error(),
		}
	}

	if !s.isLoginSuccessful(page) {
		return nil, &bank.ScraperError{
			BankCode:  bank.BankBBVA,
			Operation: "Login",
			Cause:     bank.ErrUnknown,
			Details:   "login completed but landing page unexpected",
		}
	}

	// Store session

	session := &bank.Session{
		ID:        generateSessionID(),
		BankCode:  bank.BankBBVA,
		ExpiresAt: time.Now().Add(bbvaSessionTimeout),
	}
	s.session = session

	return session, nil
}

func (s *BBVAScraper) Close() error {
	if s.browser != nil {
		return s.browser.Close()
	}
	return nil
}

func (s *BBVAScraper) isLoginSuccessful(page *rod.Page) bool {
	// Wait for DOM to stabilize after post-login JavaScript execution
	page.MustWaitDOMStable()

	_, err := page.Element(SelectorDashboard)
	return err == nil
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
