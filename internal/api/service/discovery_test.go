package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockScraper struct {
	loginSession *bank.Session
	loginErr     error
	balances     []bank.Balance
	balanceErr   error
	logoutCalled int
	closeCalled  int
}

func (m *mockScraper) Login(_ context.Context, _ map[string]string) (*bank.Session, error) {
	if m.loginErr != nil {
		return nil, m.loginErr
	}
	return m.loginSession, nil
}

func (m *mockScraper) GetBalance(_ context.Context) ([]bank.Balance, error) {
	if m.balanceErr != nil {
		return nil, m.balanceErr
	}
	return m.balances, nil
}

func (m *mockScraper) GetTransactions(_ context.Context, _ string, _ int) ([]bank.Transaction, error) {
	return nil, nil
}

func (m *mockScraper) Logout(_ context.Context) error {
	m.logoutCalled++
	return nil
}

func (m *mockScraper) Close() error {
	m.closeCalled++
	return nil
}

type mockAccountRepo struct {
	upsertedAccounts []store.Account
	upsertedCredID   uuid.UUID
	upsertErr        error
}

func (m *mockAccountRepo) Create(_ context.Context, _ *store.Account) error { return nil }
func (m *mockAccountRepo) GetByID(_ context.Context, _ uuid.UUID) (*store.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) List(_ context.Context, _ store.AccountFilter) ([]store.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) UpdateLastSynced(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockAccountRepo) UpsertBatch(_ context.Context, credentialID uuid.UUID, accounts []store.Account) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upsertedCredID = credentialID
	m.upsertedAccounts = accounts
	return nil
}

// --- Helpers ---

func validSession() *bank.Session {
	return &bank.Session{
		ID:        "discovery-session",
		Code:      bank.BankBBVA,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
}

func sampleBalances() []bank.Balance {
	return []bank.Balance{
		{
			AccountID:        "PE001101190100064607",
			Currency:         bank.CurrencyPEN,
			AvailableBalance: 123456,
			CurrentBalance:   123456,
			FetchedAt:        time.Now(),
		},
		{
			AccountID:        "PE001101190100064608",
			Currency:         bank.CurrencyUSD,
			AvailableBalance: 78900,
			CurrentBalance:   78900,
			FetchedAt:        time.Now(),
		},
	}
}

// --- Tests ---

func TestDiscoveryService_Discover_Success(t *testing.T) {
	ms := &mockScraper{
		loginSession: validSession(),
		balances:     sampleBalances(),
	}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	repo := &mockAccountRepo{}
	credID := uuid.New()

	svc := NewDiscoveryService(repo, factory, nil)

	accounts, err := svc.Discover(context.Background(), "BBVA", map[string]string{
		"company_code": "test",
		"user_code":    "test",
		"password":     "test",
	}, credID)

	require.NoError(t, err)
	require.Len(t, accounts, 2)

	// Verify first account mapping
	assert.Equal(t, "BBVA", accounts[0].BankCode)
	assert.Equal(t, "PE001101190100064607", accounts[0].AccountNumber)
	assert.Equal(t, "PEN", accounts[0].Currency)
	assert.Equal(t, store.AccountTypeChecking, accounts[0].AccountType)

	// Verify second account mapping
	assert.Equal(t, "BBVA", accounts[1].BankCode)
	assert.Equal(t, "PE001101190100064608", accounts[1].AccountNumber)
	assert.Equal(t, "USD", accounts[1].Currency)

	// Verify upsert was called with correct credential ID
	assert.Equal(t, credID, repo.upsertedCredID)
	assert.Len(t, repo.upsertedAccounts, 2)

	// Verify cleanup
	assert.Equal(t, 1, ms.logoutCalled, "should logout after discovery")
	assert.Equal(t, 1, ms.closeCalled, "should close after discovery")
}

func TestDiscoveryService_Discover_FactoryError(t *testing.T) {
	factory := func(_ bank.Code) (bank.Scraper, error) {
		return nil, errors.New("browser launch failed")
	}
	repo := &mockAccountRepo{}

	svc := NewDiscoveryService(repo, factory, nil)

	_, err := svc.Discover(context.Background(), "BBVA", map[string]string{}, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create scraper")
}

func TestDiscoveryService_Discover_LoginError(t *testing.T) {
	ms := &mockScraper{loginErr: bank.ErrInvalidCredentials}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	repo := &mockAccountRepo{}

	svc := NewDiscoveryService(repo, factory, nil)

	_, err := svc.Discover(context.Background(), "BBVA", map[string]string{
		"company_code": "bad",
		"user_code":    "bad",
		"password":     "bad",
	}, uuid.New())

	require.Error(t, err)
	assert.ErrorIs(t, err, bank.ErrInvalidCredentials)
	assert.Equal(t, 1, ms.closeCalled, "should close on login failure")
}

func TestDiscoveryService_Discover_GetBalanceError(t *testing.T) {
	ms := &mockScraper{
		loginSession: validSession(),
		balanceErr:   errors.New("page timeout"),
	}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	repo := &mockAccountRepo{}

	svc := NewDiscoveryService(repo, factory, nil)

	_, err := svc.Discover(context.Background(), "BBVA", map[string]string{
		"company_code": "test",
		"user_code":    "test",
		"password":     "test",
	}, uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get balance")
	assert.Equal(t, 1, ms.logoutCalled, "should logout on balance error")
	assert.Equal(t, 1, ms.closeCalled, "should close on balance error")
}

func TestDiscoveryService_Discover_EmptyBalances(t *testing.T) {
	ms := &mockScraper{
		loginSession: validSession(),
		balances:     []bank.Balance{},
	}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	repo := &mockAccountRepo{}

	svc := NewDiscoveryService(repo, factory, nil)

	accounts, err := svc.Discover(context.Background(), "BBVA", map[string]string{
		"company_code": "test",
		"user_code":    "test",
		"password":     "test",
	}, uuid.New())

	require.NoError(t, err)
	assert.Empty(t, accounts)
	assert.Equal(t, 1, ms.logoutCalled)
	assert.Equal(t, 1, ms.closeCalled)
}

func TestDiscoveryService_Discover_UpsertError(t *testing.T) {
	ms := &mockScraper{
		loginSession: validSession(),
		balances:     sampleBalances(),
	}
	factory := func(_ bank.Code) (bank.Scraper, error) { return ms, nil }
	repo := &mockAccountRepo{upsertErr: errors.New("db connection lost")}

	svc := NewDiscoveryService(repo, factory, nil)

	_, err := svc.Discover(context.Background(), "BBVA", map[string]string{
		"company_code": "test",
		"user_code":    "test",
		"password":     "test",
	}, uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert accounts")
	assert.Equal(t, 1, ms.logoutCalled, "should still cleanup")
	assert.Equal(t, 1, ms.closeCalled)
}
