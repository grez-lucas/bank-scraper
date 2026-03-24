// Package session provides a lazy singleton session manager for bank scrapers.
// It maintains one authenticated scraper instance per bank, creating them on
// first request and re-authenticating when sessions expire.
package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
)

// CredentialProvider retrieves decrypted bank credentials.
// Satisfied by credmgr/service.CredentialService.GetCredentials.
type CredentialProvider interface {
	GetCredentials(ctx context.Context, bankCode string) (map[string]string, error)
}

// ScraperFactory creates a new bank scraper instance for the given bank code.
type ScraperFactory func(bankCode bank.Code) (bank.Scraper, error)

// SessionInfo describes the state of a managed scraper session.
type SessionInfo struct {
	BankCode  bank.Code
	Active    bool
	ExpiresAt *time.Time
}

// managedScraper holds a scraper and its session metadata.
type managedScraper struct {
	scraper bank.Scraper
	session *bank.Session
}

// Manager manages lazy singleton scraper instances per bank.
// Thread-safe via mutex.
type Manager struct {
	mu       sync.Mutex
	scrapers map[bank.Code]*managedScraper
	creds    CredentialProvider
	factory  ScraperFactory
	logger   *slog.Logger
}

// NewManager creates a new session manager.
func NewManager(creds CredentialProvider, factory ScraperFactory, logger *slog.Logger) *Manager {
	return &Manager{
		scrapers: make(map[bank.Code]*managedScraper),
		creds:    creds,
		factory:  factory,
		logger:   logger,
	}
}

// GetScraper returns a logged-in scraper for the given bank.
// On first call, it creates and authenticates a new scraper.
// On subsequent calls, it returns the cached scraper if the session is still valid.
// If the session has expired, it closes the old scraper and creates a fresh one.
func (m *Manager) GetScraper(ctx context.Context, bankCode bank.Code) (bank.Scraper, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for existing active session
	if ms, ok := m.scrapers[bankCode]; ok {
		if time.Now().Before(ms.session.ExpiresAt) {
			return ms.scraper, nil
		}
		// Session expired — close old scraper
		m.logger.Info("session expired, creating new scraper",
			slog.String("bank", string(bankCode)),
			slog.String("session_id", ms.session.ID))
		_ = ms.scraper.Close()
		delete(m.scrapers, bankCode)
	}

	// Create new scraper
	scraper, err := m.factory(bankCode)
	if err != nil {
		return nil, fmt.Errorf("create scraper for %s: %w", bankCode, err)
	}

	// Fetch credentials
	creds, err := m.creds.GetCredentials(ctx, string(bankCode))
	if err != nil {
		_ = scraper.Close()
		return nil, fmt.Errorf("get credentials for %s: %w", bankCode, err)
	}

	// Login
	session, err := scraper.Login(ctx, creds)
	if err != nil {
		_ = scraper.Close()
		return nil, fmt.Errorf("login to %s: %w", bankCode, err)
	}

	m.scrapers[bankCode] = &managedScraper{
		scraper: scraper,
		session: session,
	}

	m.logger.Info("scraper session created",
		slog.String("bank", string(bankCode)),
		slog.String("session_id", session.ID),
		slog.Time("expires_at", session.ExpiresAt))

	return scraper, nil
}

// Invalidate removes and closes the scraper for a bank.
// The next GetScraper call will create a fresh instance.
// Use this when a scraper operation returns ErrSessionExpired.
func (m *Manager) Invalidate(bankCode bank.Code) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ms, ok := m.scrapers[bankCode]; ok {
		m.logger.Info("invalidating scraper session",
			slog.String("bank", string(bankCode)))
		_ = ms.scraper.Close()
		delete(m.scrapers, bankCode)
	}
}

// Shutdown logs out and closes all active scrapers.
// Call this on graceful server shutdown.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for code, ms := range m.scrapers {
		m.logger.Info("shutting down scraper",
			slog.String("bank", string(code)))
		_ = ms.scraper.Logout(ctx)
		_ = ms.scraper.Close()
	}
	m.scrapers = make(map[bank.Code]*managedScraper)
}

// SessionStatus returns the current state of all managed sessions.
// Used by the health endpoint to report per-bank status without triggering scraping.
func (m *Manager) SessionStatus() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	infos := make([]SessionInfo, 0, len(m.scrapers))
	for code, ms := range m.scrapers {
		exp := ms.session.ExpiresAt
		infos = append(infos, SessionInfo{
			BankCode:  code,
			Active:    time.Now().Before(exp),
			ExpiresAt: &exp,
		})
	}
	return infos
}
