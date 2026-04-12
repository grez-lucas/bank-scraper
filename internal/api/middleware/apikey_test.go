package middleware

import (
	"context"
	"crypto/sha256"
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

// --- Mock ---

type mockAPIKeyRepo struct {
	key *store.APIKey
	err error

	lastUsedCh chan uuid.UUID // signals when UpdateLastUsed is called
}

func (m *mockAPIKeyRepo) Create(_ context.Context, _ *store.APIKey) error { return nil }
func (m *mockAPIKeyRepo) Revoke(_ context.Context, _ uuid.UUID) error     { return nil }
func (m *mockAPIKeyRepo) List(_ context.Context) ([]store.APIKey, error)  { return nil, nil }

func (m *mockAPIKeyRepo) GetByKeyHash(_ context.Context, _ []byte) (*store.APIKey, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.key == nil {
		return nil, store.ErrNotFound
	}
	return m.key, nil
}

func (m *mockAPIKeyRepo) UpdateLastUsed(_ context.Context, id uuid.UUID) error {
	if m.lastUsedCh != nil {
		m.lastUsedCh <- id
	}
	return nil
}

// --- Helpers ---

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(repo store.APIKeyRepository) *gin.Engine {
	r := gin.New()
	r.Use(APIKeyAuth(repo))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"client_id": GetClientID(c),
		})
	})
	return r
}

func makeRequest(router *gin.Engine, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

type errorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func parseError(t *testing.T, w *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var resp errorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "response should be valid JSON")
	return resp
}

// --- Tests ---

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	rawKey := "test-api-key-valid-001"
	hash := sha256.Sum256([]byte(rawKey))
	keyID := uuid.New()

	ch := make(chan uuid.UUID, 1)
	repo := &mockAPIKeyRepo{
		key: &store.APIKey{
			ID:       keyID,
			KeyHash:  hash[:],
			ClientID: "aynifx",
		},
		lastUsedCh: ch,
	}

	router := setupRouter(repo)
	w := makeRequest(router, rawKey)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "aynifx", resp["client_id"])

	// Wait for async UpdateLastUsed goroutine
	select {
	case id := <-ch:
		assert.Equal(t, keyID, id)
	case <-time.After(time.Second):
		t.Fatal("UpdateLastUsed was not called within 1s")
	}
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	repo := &mockAPIKeyRepo{}
	router := setupRouter(repo)
	w := makeRequest(router, "")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "error", resp.Status)
	assert.Contains(t, resp.Message, "API key")
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	repo := &mockAPIKeyRepo{} // key is nil → returns ErrNotFound
	router := setupRouter(repo)
	w := makeRequest(router, "nonexistent-key")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "error", resp.Status)
	assert.Contains(t, resp.Message, "invalid")
}

func TestAPIKeyAuth_RevokedKey(t *testing.T) {
	rawKey := "test-api-key-revoked"
	hash := sha256.Sum256([]byte(rawKey))
	now := time.Now()

	repo := &mockAPIKeyRepo{
		key: &store.APIKey{
			ID:        uuid.New(),
			KeyHash:   hash[:],
			ClientID:  "aynifx",
			RevokedAt: &now,
		},
	}

	router := setupRouter(repo)
	w := makeRequest(router, rawKey)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "error", resp.Status)
	assert.Contains(t, resp.Message, "revoked")
}

func TestAPIKeyAuth_GetClientID_NoAuth(t *testing.T) {
	// Test the helper when middleware wasn't applied
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.Equal(t, "", GetClientID(c))
}
