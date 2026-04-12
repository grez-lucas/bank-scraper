// Package resilience provides retry and circuit breaker wrappers for scraper operations.
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/aynifx/bank-scraper/internal/scraper/bank"
)

// Config controls retry behavior.
type Config struct {
	MaxAttempts  uint64
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// DefaultConfig returns the PRD-specified retry settings (FR-801/802).
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
	}
}

// retryableErrors are transient — worth retrying.
var retryableErrors = []error{
	bank.ErrBankUnavailable,
	bank.ErrTimeout,
	bank.ErrSessionExpired,
}

// IsRetryable returns true if the error is transient and worth retrying.
func IsRetryable(err error) bool {
	for _, re := range retryableErrors {
		if errors.Is(err, re) {
			return true
		}
	}
	return false
}

// Retry executes fn with exponential backoff. Permanent errors are not retried.
func Retry[T any](ctx context.Context, cfg Config, fn func() (T, error)) (T, error) {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 1
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = cfg.InitialDelay
	b.MaxInterval = cfg.MaxDelay
	b.MaxElapsedTime = 0 // controlled by max retries, not elapsed time

	bCtx := backoff.WithContext(
		backoff.WithMaxRetries(b, cfg.MaxAttempts-1),
		ctx,
	)

	var result T
	err := backoff.Retry(func() error {
		var opErr error
		result, opErr = fn()
		if opErr == nil {
			return nil
		}
		if !IsRetryable(opErr) {
			return backoff.Permanent(opErr)
		}
		return opErr
	}, bCtx)

	return result, err
}
