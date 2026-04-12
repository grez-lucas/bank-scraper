package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/aynifx/bank-scraper/internal/scraper/bank/banktest"
	"github.com/aynifx/bank-scraper/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockScraperProvider struct {
	scraper bank.Scraper
	err     error
}

func (m *mockScraperProvider) GetScraper(_ context.Context, _ bank.Code) (bank.Scraper, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.scraper, nil
}

func (m *mockScraperProvider) Invalidate(_ bank.Code) {}

// --- Helpers ---

func testAccount() store.Account {
	return store.Account{
		ID:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		BankCode:      "BBVA",
		AccountNumber: "PE001101190100064607",
		Currency:      "PEN",
		AccountType:   store.AccountTypeChecking,
		Status:        store.AccountStatusActive,
	}
}

func setupBalanceRouter(accounts store.AccountRepository, scraper *mockScraperProvider) *gin.Engine {
	r := gin.New()
	h := NewBalanceHandler(accounts, scraper)
	r.GET("/api/v1/accounts/:account_id/balance", h.Get)
	return r
}

// --- Tests ---

func TestBalanceHandler_Get_Success(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	ms := &banktest.MockScraper{
		Balances: []bank.Balance{
			{
				AccountID:        "PE001101190100064607",
				Currency:         bank.CurrencyPEN,
				AvailableBalance: 123456,
				CurrentBalance:   123400,
				FetchedAt:        time.Now(),
			},
			{
				AccountID:        "PE001101190100064608",
				Currency:         bank.CurrencyUSD,
				AvailableBalance: 78900,
				CurrentBalance:   78900,
				FetchedAt:        time.Now(),
			},
		},
	}
	sp := &mockScraperProvider{scraper: ms}

	router := setupBalanceRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp BalanceResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, acct.ID.String(), resp.AccountID)
	assert.Equal(t, "BBVA", resp.BankCode)
	assert.Equal(t, "PEN", resp.Currency)
	assert.Equal(t, "1234.56", resp.AvailableBalance)
	assert.Equal(t, "1234.00", resp.CurrentBalance)
}

func TestBalanceHandler_Get_AccountNotFound(t *testing.T) {
	repo := &mockAccountRepo{accounts: []store.Account{}}
	sp := &mockScraperProvider{}

	router := setupBalanceRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.New().String()+"/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBalanceHandler_Get_InvalidAccountID(t *testing.T) {
	repo := &mockAccountRepo{}
	sp := &mockScraperProvider{}

	router := setupBalanceRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/not-a-uuid/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBalanceHandler_Get_ScraperError(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	sp := &mockScraperProvider{err: errors.New("browser crashed")}

	router := setupBalanceRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestBalanceHandler_Get_BalanceNotInResults(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	ms := &banktest.MockScraper{
		Balances: []bank.Balance{
			{AccountID: "DIFFERENT_ACCOUNT", Currency: bank.CurrencyUSD, AvailableBalance: 100},
		},
	}
	sp := &mockScraperProvider{scraper: ms}

	router := setupBalanceRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
