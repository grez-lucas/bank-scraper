package store

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash := sha256.Sum256([]byte("test-api-key-001"))

	k := &APIKey{
		KeyHash:     hash[:],
		ClientID:    "aynifx",
		Description: strPtr("Production key"),
	}

	err := repo.Create(ctx, k)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, k.ID)
	assert.False(t, k.CreatedAt.IsZero())
	assert.Nil(t, k.RevokedAt)
	assert.Nil(t, k.LastUsedAt)
}

func TestAPIKeyRepo_GetByKeyHash(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash := sha256.Sum256([]byte("test-api-key-002"))
	k := createTestAPIKey(t, repo, hash[:], "aynifx")

	fetched, err := repo.GetByKeyHash(ctx, hash[:])
	require.NoError(t, err)

	assert.Equal(t, k.ID, fetched.ID)
	assert.Equal(t, "aynifx", fetched.ClientID)
	assert.Equal(t, hash[:], fetched.KeyHash)
}

func TestAPIKeyRepo_GetByKeyHash_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash := sha256.Sum256([]byte("nonexistent-key"))

	_, err := repo.GetByKeyHash(ctx, hash[:])
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAPIKeyRepo_Revoke(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash := sha256.Sum256([]byte("test-api-key-003"))
	k := createTestAPIKey(t, repo, hash[:], "aynifx")

	err := repo.Revoke(ctx, k.ID)
	require.NoError(t, err)

	fetched, err := repo.GetByKeyHash(ctx, hash[:])
	require.NoError(t, err)
	assert.NotNil(t, fetched.RevokedAt)
	assert.WithinDuration(t, time.Now(), *fetched.RevokedAt, 5*time.Second)
}

func TestAPIKeyRepo_Revoke_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	err := repo.Revoke(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAPIKeyRepo_UpdateLastUsed(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash := sha256.Sum256([]byte("test-api-key-004"))
	k := createTestAPIKey(t, repo, hash[:], "aynifx")
	assert.Nil(t, k.LastUsedAt)

	err := repo.UpdateLastUsed(ctx, k.ID)
	require.NoError(t, err)

	fetched, err := repo.GetByKeyHash(ctx, hash[:])
	require.NoError(t, err)
	assert.NotNil(t, fetched.LastUsedAt)
	assert.WithinDuration(t, time.Now(), *fetched.LastUsedAt, 5*time.Second)
}

func TestAPIKeyRepo_List(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewAPIKeyRepo(pool)

	ctx := context.Background()
	hash1 := sha256.Sum256([]byte("test-api-key-005"))
	hash2 := sha256.Sum256([]byte("test-api-key-006"))
	createTestAPIKey(t, repo, hash1[:], "aynifx")
	createTestAPIKey(t, repo, hash2[:], "other-client")

	keys, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

// createTestAPIKey inserts a minimal API key for tests.
func createTestAPIKey(t *testing.T, repo *APIKeyRepo, keyHash []byte, clientID string) *APIKey {
	t.Helper()

	k := &APIKey{
		KeyHash:     keyHash,
		ClientID:    clientID,
		Description: strPtr("Test key"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := repo.Create(ctx, k); err != nil {
		t.Fatalf("create test api key: %v", err)
	}
	return k
}

func strPtr(s string) *string {
	return &s
}
