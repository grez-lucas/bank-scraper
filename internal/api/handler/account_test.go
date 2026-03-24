package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- Mock ---

type mockAccountRepo struct {
	accounts []store.Account
	err      error
	filter   store.AccountFilter // captures last filter
}

func (m *mockAccountRepo) Create(_ context.Context, _ *store.Account) error { return nil }
func (m *mockAccountRepo) GetByID(_ context.Context, id uuid.UUID) (*store.Account, error) {
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			return &m.accounts[i], nil
		}
	}
	return nil, store.ErrNotFound
}
func (m *mockAccountRepo) List(_ context.Context, filter store.AccountFilter) ([]store.Account, error) {
	m.filter = filter
	if m.err != nil {
		return nil, m.err
	}
	return m.accounts, nil
}
func (m *mockAccountRepo) UpsertBatch(_ context.Context, _ uuid.UUID, _ []store.Account) error {
	return nil
}
func (m *mockAccountRepo) UpdateLastSynced(_ context.Context, _ uuid.UUID) error { return nil }

// --- Helpers ---

func sampleAccounts() []store.Account {
	now := time.Now()
	return []store.Account{
		{
			ID:            uuid.New(),
			BankCode:      "BBVA",
			AccountNumber: "PE001101190100064607",
			Currency:      "PEN",
			AccountType:   store.AccountTypeChecking,
			Status:        store.AccountStatusActive,
			LastSyncedAt:  &now,
		},
		{
			ID:            uuid.New(),
			BankCode:      "BBVA",
			AccountNumber: "PE001101190100064608",
			Currency:      "USD",
			AccountType:   store.AccountTypeChecking,
			Status:        store.AccountStatusActive,
			LastSyncedAt:  &now,
		},
	}
}

func setupAccountRouter(repo store.AccountRepository) *gin.Engine {
	r := gin.New()
	h := NewAccountHandler(repo)
	r.GET("/api/v1/accounts", h.List)
	return r
}

// --- Tests ---

func TestAccountHandler_List(t *testing.T) {
	repo := &mockAccountRepo{accounts: sampleAccounts()}
	router := setupAccountRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Accounts []AccountResponse `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Accounts, 2)

	// Account numbers should be masked
	assert.Equal(t, "XXXXXXXXXXXXXXXX4607", resp.Accounts[0].AccountNumber)
	assert.Equal(t, "BBVA", resp.Accounts[0].BankCode)
	assert.Equal(t, "PEN", resp.Accounts[0].Currency)
}

func TestAccountHandler_List_FilterByBankCode(t *testing.T) {
	repo := &mockAccountRepo{accounts: sampleAccounts()}
	router := setupAccountRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?bank_code=BBVA", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.filter.BankCode)
	assert.Equal(t, "BBVA", *repo.filter.BankCode)
}

func TestAccountHandler_List_FilterByCurrency(t *testing.T) {
	repo := &mockAccountRepo{accounts: sampleAccounts()}
	router := setupAccountRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?currency=USD", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.filter.Currency)
	assert.Equal(t, "USD", *repo.filter.Currency)
}

func TestAccountHandler_List_Empty(t *testing.T) {
	repo := &mockAccountRepo{accounts: []store.Account{}}
	router := setupAccountRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Accounts []AccountResponse `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Accounts)
}
