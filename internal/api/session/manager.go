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

	"github.com/aynifx/bank-scraper/internal/scraper/bank"
)

// CredentialProvider retrieves decrypted bank credentials.
// Satisfied by credmgr/service.CredentialService.GetCredentials.
type CredentialProvider interface {
	GetCredentials(ctx context.Context, bankCode string) (map[string]string, error)
}

// Info describes the state of a managed scraper session.
type Info struct {
	BankCode  bank.Code
	Active    bool
	ExpiresAt time.Time
}

// managedScraper holds a scraper and its session metadata.
type managedScraper struct {
	scraper bank.Scraper
	session *bank.Session
}

// Manager manages lazy singleton scraper instances per bank.
// Uses per-bank locking so Login for one bank doesn't block cached lookups for others.
type Manager struct {
	mu       sync.Mutex                    // protects the scrapers map itself
	scrapers map[bank.Code]*managedScraper // cached scraper instances
	locks    map[bank.Code]*sync.Mutex     // per-bank lock for Login serialization
	creds    CredentialProvider
	factory  bank.ScraperFactory
	logger   *slog.Logger
}

// NewManager creates a new session manager.
func NewManager(creds CredentialProvider, factory bank.ScraperFactory, logger *slog.Logger) *Manager {
	return &Manager{
		scrapers: make(map[bank.Code]*managedScraper),
		locks:    make(map[bank.Code]*sync.Mutex),
		creds:    creds,
		factory:  factory,
		logger:   logger,
	}
}

// bankLock returns (or creates) the per-bank mutex.
func (m *Manager) bankLock(bankCode bank.Code) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.locks[bankCode]; !ok {
		m.locks[bankCode] = &sync.Mutex{}
	}
	return m.locks[bankCode]
}

// GetScraper returns a logged-in scraper for the given bank.
// On first call, it creates and authenticates a new scraper.
// On subsequent calls, it returns the cached scraper if the session is still valid.
// If the session has expired, it closes the old scraper and creates a fresh one.
//
// Uses per-bank locking so a slow Login for one bank doesn't block cached lookups for others.
func (m *Manager) GetScraper(ctx context.Context, bankCode bank.Code) (bank.Scraper, error) {
	bl := m.bankLock(bankCode)
	bl.Lock()
	defer bl.Unlock()

	// Check for existing active session (brief map read under global lock)
	m.mu.Lock()
	ms, ok := m.scrapers[bankCode]
	m.mu.Unlock()

	if ok && time.Now().Before(ms.session.ExpiresAt) {
		return ms.scraper, nil
	}

	// Session expired or doesn't exist — clean up old one if present
	if ok {
		m.logger.Info("session expired, creating new scraper",
			slog.String("bank", string(bankCode)),
			slog.String("session_id", ms.session.ID))
		_ = ms.scraper.Close()
		m.mu.Lock()
		delete(m.scrapers, bankCode)
		m.mu.Unlock()
	}

	// Create new scraper (slow: launches browser)
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

	// Login (slow: 15-20s for BBVA)
	session, err := scraper.Login(ctx, creds)
	if err != nil {
		_ = scraper.Close()
		return nil, fmt.Errorf("login to %s: %w", bankCode, err)
	}

	m.mu.Lock()
	m.scrapers[bankCode] = &managedScraper{
		scraper: scraper,
		session: session,
	}
	m.mu.Unlock()

	m.logger.Info("scraper session created",
		slog.String("bank", string(bankCode)),
		slog.String("session_id", session.ID),
		slog.Time("expires_at", session.ExpiresAt))

	return scraper, nil
}

// Invalidate removes and closes the scraper for a bank.
// The next GetScraper call will create a fresh instance.
func (m *Manager) Invalidate(bankCode bank.Code) {
	m.mu.Lock()
	ms, ok := m.scrapers[bankCode]
	if ok {
		delete(m.scrapers, bankCode)
	}
	m.mu.Unlock()

	if ok {
		m.logger.Info("invalidating scraper session",
			slog.String("bank", string(bankCode)))
		_ = ms.scraper.Close()
	}
}

// Shutdown logs out and closes all active scrapers.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	scrapers := m.scrapers
	m.scrapers = make(map[bank.Code]*managedScraper)
	m.mu.Unlock()

	for code, ms := range scrapers {
		m.logger.Info("shutting down scraper",
			slog.String("bank", string(code)))
		_ = ms.scraper.Logout(ctx)
		_ = ms.scraper.Close()
	}
}

// SessionStatus returns the current state of all managed sessions.
func (m *Manager) SessionStatus() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()

	infos := make([]Info, 0, len(m.scrapers))
	for code, ms := range m.scrapers {
		infos = append(infos, Info{
			BankCode:  code,
			Active:    time.Now().Before(ms.session.ExpiresAt),
			ExpiresAt: ms.session.ExpiresAt,
		})
	}
	return infos
}
