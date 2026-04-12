package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockCredentialProvider struct {
	creds map[string]string
	err   error
}

func (m *mockCredentialProvider) GetCredentials(_ context.Context, _ string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.creds, nil
}

type mockCredentialRepo struct {
	cred *store.BankCredential
	err  error
}

func (m *mockCredentialRepo) Create(_ context.Context, _ *store.BankCredential) error { return nil }
func (m *mockCredentialRepo) GetByID(_ context.Context, _ uuid.UUID) (*store.BankCredential, error) {
	return nil, nil
}
func (m *mockCredentialRepo) List(_ context.Context) ([]store.BankCredential, error)  { return nil, nil }
func (m *mockCredentialRepo) Update(_ context.Context, _ *store.BankCredential) error { return nil }
func (m *mockCredentialRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error      { return nil }
func (m *mockCredentialRepo) HardDeleteExpired(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

func (m *mockCredentialRepo) GetActiveByBankCode(_ context.Context, _ string) (*store.BankCredential, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cred, nil
}

type mockDiscoverer struct {
	accounts []store.Account
	err      error
}

func (m *mockDiscoverer) Discover(_ context.Context, _ string, _ map[string]string, _ uuid.UUID) ([]store.Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.accounts, nil
}

// --- Tests ---

func setupDiscoveryRouter(h *DiscoveryHandler) *gin.Engine {
	r := gin.New()
	r.POST("/api/v1/admin/discover/:bank_code", h.Trigger)
	return r
}

func TestDiscoveryHandler_Trigger_Success(t *testing.T) {
	credID := uuid.New()
	h := NewDiscoveryHandler(
		&mockDiscoverer{accounts: []store.Account{
			{ID: uuid.New(), BankCode: "BBVA", AccountNumber: "PE001", Currency: "PEN", AccountType: store.AccountTypeChecking, Status: store.AccountStatusActive},
		}},
		&mockCredentialProvider{creds: map[string]string{"company_code": "x", "user_code": "y", "password": "z"}},
		&mockCredentialRepo{cred: &store.BankCredential{ID: credID, BankCode: "BBVA"}},
	)

	router := setupDiscoveryRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/discover/BBVA", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Accounts []AccountResponse `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Accounts, 1)
	assert.Equal(t, "BBVA", resp.Accounts[0].BankCode)
}

func TestDiscoveryHandler_Trigger_NoCredential(t *testing.T) {
	h := NewDiscoveryHandler(
		&mockDiscoverer{},
		&mockCredentialProvider{err: errors.New("no credential configured")},
		&mockCredentialRepo{err: store.ErrNotFound},
	)

	router := setupDiscoveryRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/discover/BBVA", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDiscoveryHandler_Trigger_DiscoveryError(t *testing.T) {
	credID := uuid.New()
	h := NewDiscoveryHandler(
		&mockDiscoverer{err: errors.New("login failed")},
		&mockCredentialProvider{creds: map[string]string{"company_code": "x", "user_code": "y", "password": "z"}},
		&mockCredentialRepo{cred: &store.BankCredential{ID: credID, BankCode: "BBVA"}},
	)

	router := setupDiscoveryRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/discover/BBVA", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
