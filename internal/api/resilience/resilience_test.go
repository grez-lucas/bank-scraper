package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/aynifx/bank-scraper/internal/scraper/bank/banktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockScraperProvider struct {
	scraper  bank.Scraper
	err      error
	getCalls int
	invCalls int
	getFunc  func(context.Context, bank.Code) (bank.Scraper, error) // optional override
}

func (m *mockScraperProvider) GetScraper(ctx context.Context, code bank.Code) (bank.Scraper, error) {
	m.getCalls++
	if m.getFunc != nil {
		return m.getFunc(ctx, code)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.scraper, nil
}

func (m *mockScraperProvider) Invalidate(_ bank.Code) {
	m.invCalls++
}

// --- Error classification tests ---

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"bank unavailable", bank.ErrBankUnavailable, true},
		{"timeout", bank.ErrTimeout, true},
		{"session expired", bank.ErrSessionExpired, true},
		{"wrapped retryable", &bank.ScraperError{Cause: bank.ErrBankUnavailable}, true},
		{"invalid credentials", bank.ErrInvalidCredentials, false},
		{"bot detection", bank.ErrBotDetection, false},
		{"account not found", bank.ErrAccountNotFound, false},
		{"parsing failed", bank.ErrParsingFailed, false},
		{"unknown error", errors.New("something else"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, IsRetryable(tt.err), "IsRetryable(%v)", tt.err)
		})
	}
}

// --- Retry tests ---

func TestRetry_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	result, err := Retry(context.Background(), Config{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() ([]bank.Balance, error) {
		calls++
		return []bank.Balance{{AccountID: "acc1"}}, nil
	})

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, 1, calls)
}

func TestRetry_SucceedsAfterTransientError(t *testing.T) {
	calls := 0
	result, err := Retry(context.Background(), Config{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() ([]bank.Balance, error) {
		calls++
		if calls < 3 {
			return nil, bank.ErrBankUnavailable
		}
		return []bank.Balance{{AccountID: "acc1"}}, nil
	})

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, 3, calls)
}

func TestRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	_, err := Retry(context.Background(), Config{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() ([]bank.Balance, error) {
		calls++
		return nil, bank.ErrBankUnavailable
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, bank.ErrBankUnavailable)
	assert.Equal(t, 3, calls)
}

func TestRetry_PermanentErrorNotRetried(t *testing.T) {
	calls := 0
	_, err := Retry(context.Background(), Config{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() ([]bank.Balance, error) {
		calls++
		return nil, bank.ErrInvalidCredentials
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, bank.ErrInvalidCredentials)
	assert.Equal(t, 1, calls, "permanent error should not be retried")
}

func TestRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Retry(ctx, Config{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() ([]bank.Balance, error) {
		return nil, bank.ErrBankUnavailable
	})

	require.Error(t, err)
}

// --- Circuit breaker tests ---

func TestCircuitBreakerRegistry_GetOrCreate(t *testing.T) {
	reg := NewBreakerRegistry(BreakerConfig{
		MaxFailures:  5,
		ResetTimeout: time.Minute,
	})

	cb1 := reg.Get(bank.BankBBVA)
	cb2 := reg.Get(bank.BankBBVA)
	cb3 := reg.Get(bank.BankInterbank)

	assert.Same(t, cb1, cb2, "same bank should return same breaker")
	assert.NotSame(t, cb1, cb3, "different banks should have different breakers")
}

func TestCircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	reg := NewBreakerRegistry(BreakerConfig{
		MaxFailures:  3,
		ResetTimeout: time.Minute,
	})

	cb := reg.Get(bank.BankBBVA)

	// Fail 3 times to trip the breaker
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return nil, errors.New("bank down")
		})
		require.Error(t, err)
	}

	// Next call should fail fast (circuit open)
	_, err := cb.Execute(func() (interface{}, error) {
		t.Fatal("should not be called when circuit is open")
		return nil, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
}

// --- Integration: ResilientScraperProvider ---

func TestResilientProvider_GetScraper_Success(t *testing.T) {
	ms := &banktest.MockScraper{
		Balances: []bank.Balance{{AccountID: "acc1"}},
	}
	inner := &mockScraperProvider{scraper: ms}
	cfg := Config{MaxAttempts: 3, InitialDelay: time.Millisecond}
	rp := NewResilientProvider(inner, cfg, NewBreakerRegistry(DefaultBreakerConfig()))

	scraper, err := rp.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.NotNil(t, scraper)
	assert.Equal(t, 1, inner.getCalls, "should call inner once on success")
}

func TestResilientProvider_GetScraper_RetriesTransientError(t *testing.T) {
	ms := &banktest.MockScraper{}
	calls := 0
	inner := &mockScraperProvider{}
	// Override GetScraper to fail twice then succeed
	inner.getFunc = func(_ context.Context, _ bank.Code) (bank.Scraper, error) {
		calls++
		if calls < 3 {
			return nil, bank.ErrBankUnavailable
		}
		return ms, nil
	}
	cfg := Config{MaxAttempts: 3, InitialDelay: time.Millisecond}
	rp := NewResilientProvider(inner, cfg, NewBreakerRegistry(DefaultBreakerConfig()))

	scraper, err := rp.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.NotNil(t, scraper)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 2, inner.invCalls, "should invalidate on each transient error")
}

func TestResilientProvider_GetScraper_CircuitBreakerOpens(t *testing.T) {
	inner := &mockScraperProvider{err: bank.ErrBankUnavailable}
	cfg := Config{MaxAttempts: 1, InitialDelay: time.Millisecond}
	breakers := NewBreakerRegistry(BreakerConfig{MaxFailures: 2, ResetTimeout: time.Minute})
	rp := NewResilientProvider(inner, cfg, breakers)

	// Fail twice to trip the breaker
	_, _ = rp.GetScraper(context.Background(), bank.BankBBVA)
	_, _ = rp.GetScraper(context.Background(), bank.BankBBVA)

	// Third call should fail fast (circuit open) without hitting inner
	prevCalls := inner.getCalls
	_, err := rp.GetScraper(context.Background(), bank.BankBBVA)
	require.Error(t, err)
	assert.Equal(t, prevCalls, inner.getCalls, "should not call inner when circuit is open")
}

func TestResilientProvider_Invalidate_PassesThrough(t *testing.T) {
	inner := &mockScraperProvider{}
	rp := NewResilientProvider(inner, DefaultConfig(), NewBreakerRegistry(DefaultBreakerConfig()))

	rp.Invalidate(bank.BankBBVA)
	assert.Equal(t, 1, inner.invCalls)
}
