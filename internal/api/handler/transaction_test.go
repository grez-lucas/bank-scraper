package handler

import (
	"encoding/json"
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

func sampleTransactions() []bank.Transaction {
	bal := int64(500000)
	return []bank.Transaction{
		{
			ID:           "DOC001",
			Date:         time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			Description:  "Transfer received",
			Amount:       100000,
			Type:         bank.TransactionCredit,
			BalanceAfter: &bal,
		},
		{
			ID:          "DOC002",
			Date:        time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			Description: "Payment sent",
			Amount:      50000,
			Type:        bank.TransactionDebit,
		},
	}
}

func setupTransactionRouter(accounts store.AccountRepository, scraper *mockScraperProvider) *gin.Engine {
	r := gin.New()
	h := NewTransactionHandler(accounts, scraper)
	r.GET("/api/v1/accounts/:account_id/transactions", h.List)
	return r
}

func TestTransactionHandler_List_Success(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	ms := &banktest.MockScraper{Transactions: sampleTransactions()}
	sp := &mockScraperProvider{scraper: ms}

	router := setupTransactionRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/transactions?from_date=2026-03-15&to_date=2026-03-25", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp TransactionsListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, acct.ID.String(), resp.AccountID)
	assert.Equal(t, "BBVA", resp.BankCode)
	assert.Len(t, resp.Transactions, 2)
	assert.Equal(t, "1000.00", resp.Transactions[0].Amount)
	assert.Equal(t, "CREDIT", resp.Transactions[0].Type)
	assert.NotNil(t, resp.Transactions[0].BalanceAfter)
	assert.Equal(t, "5000.00", *resp.Transactions[0].BalanceAfter)
	assert.Nil(t, resp.Transactions[1].BalanceAfter)
}

func TestTransactionHandler_List_DefaultDates(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	ms := &banktest.MockScraper{Transactions: []bank.Transaction{}}
	sp := &mockScraperProvider{scraper: ms}

	router := setupTransactionRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp TransactionsListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Default: last 7 days
	toDate, _ := time.Parse("2006-01-02", resp.ToDate)
	fromDate, _ := time.Parse("2006-01-02", resp.FromDate)
	assert.InDelta(t, 7, toDate.Sub(fromDate).Hours()/24, 1)
}

func TestTransactionHandler_List_CustomDates(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	ms := &banktest.MockScraper{Transactions: []bank.Transaction{}}
	sp := &mockScraperProvider{scraper: ms}

	router := setupTransactionRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/transactions?from_date=2026-03-01&to_date=2026-03-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp TransactionsListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2026-03-01", resp.FromDate)
	assert.Equal(t, "2026-03-15", resp.ToDate)
}

func TestTransactionHandler_List_DateRangeTooLarge(t *testing.T) {
	acct := testAccount()
	repo := &mockAccountRepo{accounts: []store.Account{acct}}
	sp := &mockScraperProvider{}

	router := setupTransactionRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+acct.ID.String()+"/transactions?from_date=2025-01-01&to_date=2026-03-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTransactionHandler_List_AccountNotFound(t *testing.T) {
	repo := &mockAccountRepo{accounts: []store.Account{}}
	sp := &mockScraperProvider{}

	router := setupTransactionRouter(repo, sp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.New().String()+"/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
