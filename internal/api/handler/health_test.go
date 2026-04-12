package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/aynifx/bank-scraper/internal/api/session"
	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ interface{}) error {
	return m.err
}

type mockSessionStatus struct {
	infos []session.Info
}

func (m *mockSessionStatus) SessionStatus() []session.Info {
	return m.infos
}

// --- Tests ---

func TestHealthHandler_Check_Healthy(t *testing.T) {
	h := NewHealthHandler(
		func() error { return nil },
		&mockSessionStatus{
			infos: []session.Info{
				{BankCode: bank.BankBBVA, Active: true, ExpiresAt: time.Now().Add(5 * time.Minute)},
			},
		},
	)

	r := gin.New()
	r.GET("/health", h.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, StatusHealthy, resp.Status)
	assert.Contains(t, resp.Banks, "BBVA")
	assert.Equal(t, StatusHealthy, resp.Banks["BBVA"].Status)
}

func TestHealthHandler_Check_Degraded_DBDown(t *testing.T) {
	h := NewHealthHandler(
		func() error { return errors.New("connection refused") },
		&mockSessionStatus{},
	)

	r := gin.New()
	r.GET("/health", h.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, StatusDegraded, resp.Status)
}

func TestHealthHandler_Check_NoSessions(t *testing.T) {
	h := NewHealthHandler(
		func() error { return nil },
		&mockSessionStatus{},
	)

	r := gin.New()
	r.GET("/health", h.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, StatusHealthy, resp.Status)
	assert.Empty(t, resp.Banks)
}

func TestHealthHandler_Check_ExpiredSession(t *testing.T) {
	h := NewHealthHandler(
		func() error { return nil },
		&mockSessionStatus{
			infos: []session.Info{
				{BankCode: bank.BankBBVA, Active: false, ExpiresAt: time.Now().Add(-1 * time.Minute)},
			},
		},
	)

	r := gin.New()
	r.GET("/health", h.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, StatusHealthy, resp.Status)
	assert.Equal(t, StatusDegraded, resp.Banks["BBVA"].Status)
}
