package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/sony/gobreaker"
)

// BreakerConfig controls circuit breaker behavior.
type BreakerConfig struct {
	MaxFailures  uint32        // consecutive failures before opening (FR-805: 5)
	ResetTimeout time.Duration // time before half-open attempt (FR-806: 5min)
}

// DefaultBreakerConfig returns the PRD-specified circuit breaker settings.
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		MaxFailures:  5,
		ResetTimeout: 5 * time.Minute,
	}
}

// BreakerRegistry manages per-bank circuit breakers.
type BreakerRegistry struct {
	mu       sync.Mutex
	breakers map[bank.Code]*gobreaker.CircuitBreaker
	cfg      BreakerConfig
}

// NewBreakerRegistry creates a registry with the given configuration.
func NewBreakerRegistry(cfg BreakerConfig) *BreakerRegistry {
	return &BreakerRegistry{
		breakers: make(map[bank.Code]*gobreaker.CircuitBreaker),
		cfg:      cfg,
	}
}

// Get returns the circuit breaker for the given bank, creating it if needed.
func (r *BreakerRegistry) Get(bankCode bank.Code) *gobreaker.CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, ok := r.breakers[bankCode]; ok {
		return cb
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    fmt.Sprintf("bank-%s", bankCode),
		Timeout: r.cfg.ResetTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= r.cfg.MaxFailures
		},
	})
	r.breakers[bankCode] = cb
	return cb
}

// ScraperProvider matches the handler.ScraperProvider interface.
type ScraperProvider interface {
	GetScraper(ctx context.Context, bankCode bank.Code) (bank.Scraper, error)
	Invalidate(bankCode bank.Code)
}

// ResilientProvider wraps a ScraperProvider with retry and circuit breaker.
// It satisfies handler.ScraperProvider so handlers get resilience transparently.
type ResilientProvider struct {
	inner    ScraperProvider
	retryCfg Config
	breakers *BreakerRegistry
}

// NewResilientProvider creates a resilient wrapper around an existing ScraperProvider.
func NewResilientProvider(inner ScraperProvider, retryCfg Config, breakers *BreakerRegistry) *ResilientProvider {
	return &ResilientProvider{
		inner:    inner,
		retryCfg: retryCfg,
		breakers: breakers,
	}
}

// GetScraper returns a logged-in scraper, guarded by the per-bank circuit breaker.
// When the circuit is open (bank known-down), this fails fast without attempting login.
// Retries transient errors (ErrBankUnavailable, ErrTimeout) with exponential backoff.
// On ErrSessionExpired, invalidates the session and retries with a fresh login.
func (r *ResilientProvider) GetScraper(ctx context.Context, bankCode bank.Code) (bank.Scraper, error) {
	cb := r.breakers.Get(bankCode)

	result, err := cb.Execute(func() (interface{}, error) {
		scraper, err := Retry(ctx, r.retryCfg, func() (bank.Scraper, error) {
			s, err := r.inner.GetScraper(ctx, bankCode)
			if err != nil && IsRetryable(err) {
				r.inner.Invalidate(bankCode)
			}
			return s, err
		})
		return scraper, err
	})
	if err != nil {
		return nil, err
	}
	return result.(bank.Scraper), nil
}

// Invalidate passes through to the inner provider.
func (r *ResilientProvider) Invalidate(bankCode bank.Code) {
	r.inner.Invalidate(bankCode)
}
