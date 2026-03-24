package session

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/banktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks (only session-manager-specific ones; scraper mock is shared) ---

type mockCredProvider struct {
	creds map[string]string
	err   error
}

func (m *mockCredProvider) GetCredentials(_ context.Context, _ string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.creds, nil
}

// --- Helpers ---

func validSession() *bank.Session {
	return &bank.Session{
		ID:        "test-session-1",
		Code:      bank.BankBBVA,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
}

func expiredSession() *bank.Session {
	return &bank.Session{
		ID:        "test-session-expired",
		Code:      bank.BankBBVA,
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
}

func validCreds() map[string]string {
	return map[string]string{
		"company_code": "test-company",
		"user_code":    "test-user",
		"password":     "test-pass",
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}

// --- Tests ---

func TestManager_GetScraper_CreatesOnFirstCall(t *testing.T) {
	ms := &banktest.MockScraper{LoginSession: validSession()}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	scraper, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.NotNil(t, scraper)
	assert.Equal(t, 1, ms.LoginCalled, "should have called Login once")
}

func TestManager_GetScraper_ReusesActiveSession(t *testing.T) {
	ms := &banktest.MockScraper{LoginSession: validSession()}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	s1, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)

	s2, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)

	assert.Same(t, s1, s2, "should return the same scraper instance")
	assert.Equal(t, 1, ms.LoginCalled, "should only login once")
}

func TestManager_GetScraper_RelogsOnExpiredSession(t *testing.T) {
	callCount := 0
	factory := func(_ bank.Code) (bank.Scraper, error) {
		callCount++
		if callCount == 1 {
			return &banktest.MockScraper{LoginSession: expiredSession()}, nil
		}
		return &banktest.MockScraper{LoginSession: validSession()}, nil
	}
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	s1, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.NotNil(t, s1)

	s2, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.NotNil(t, s2)
	assert.NotSame(t, s1, s2, "should be a different scraper after expiry")
	assert.Equal(t, 2, callCount, "factory should be called twice")
}

func TestManager_GetScraper_CredentialError(t *testing.T) {
	credErr := errors.New("no credential configured")
	factory := func(_ bank.Code) (bank.Scraper, error) {
		return &banktest.MockScraper{LoginSession: validSession()}, nil
	}
	creds := &mockCredProvider{err: credErr}

	mgr := NewManager(creds, factory, testLogger())

	scraper, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.Error(t, err)
	assert.Nil(t, scraper)
	assert.Contains(t, err.Error(), "get credentials")
}

func TestManager_GetScraper_FactoryError(t *testing.T) {
	factoryErr := errors.New("browser launch failed")
	factory := func(_ bank.Code) (bank.Scraper, error) { return nil, factoryErr }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	scraper, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.Error(t, err)
	assert.Nil(t, scraper)
	assert.Contains(t, err.Error(), "create scraper")
}

func TestManager_GetScraper_LoginError(t *testing.T) {
	ms := &banktest.MockScraper{LoginErr: bank.ErrInvalidCredentials}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	scraper, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.Error(t, err)
	assert.Nil(t, scraper)
	assert.ErrorIs(t, err, bank.ErrInvalidCredentials)
	assert.Equal(t, 1, ms.CloseCalled, "should close scraper on login failure")
}

func TestManager_Invalidate(t *testing.T) {
	callCount := 0
	factory := func(_ bank.Code) (bank.Scraper, error) {
		callCount++
		return &banktest.MockScraper{LoginSession: validSession()}, nil
	}
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	_, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	mgr.Invalidate(bank.BankBBVA)

	_, err = mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "should create a new scraper after invalidation")
}

func TestManager_Shutdown(t *testing.T) {
	ms := &banktest.MockScraper{LoginSession: validSession()}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	_, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)

	mgr.Shutdown(context.Background())

	assert.Equal(t, 1, ms.LogoutCalled, "should logout on shutdown")
	assert.Equal(t, 1, ms.CloseCalled, "should close on shutdown")
}

func TestManager_SessionStatus(t *testing.T) {
	ms := &banktest.MockScraper{LoginSession: validSession()}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	statuses := mgr.SessionStatus()
	assert.Empty(t, statuses)

	_, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)

	statuses = mgr.SessionStatus()
	require.Len(t, statuses, 1)
	assert.Equal(t, bank.BankBBVA, statuses[0].BankCode)
	assert.True(t, statuses[0].Active)
	assert.False(t, statuses[0].ExpiresAt.IsZero())
}

func TestManager_SessionStatus_ExpiredSession(t *testing.T) {
	ms := &banktest.MockScraper{LoginSession: expiredSession()}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	creds := &mockCredProvider{creds: validCreds()}

	mgr := NewManager(creds, factory, testLogger())

	_, err := mgr.GetScraper(context.Background(), bank.BankBBVA)
	require.NoError(t, err)

	statuses := mgr.SessionStatus()
	require.Len(t, statuses, 1)
	assert.False(t, statuses[0].Active, "expired session should not be active")
}
