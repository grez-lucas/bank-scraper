package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	sessionRepo := NewSessionRepo(pool)

	u := createTestUser(t, userRepo)

	s := &Session{
		UserID:    u.ID,
		TokenHash: hashToken("test-token-1"),
		IPAddress: "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		ExpiresAt: time.Now().Add(15 * time.Minute).Truncate(time.Microsecond),
	}

	ctx := context.Background()
	err := sessionRepo.Create(ctx, s)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, s.ID)
	assert.False(t, s.LastActive.IsZero())
	assert.False(t, s.CreatedAt.IsZero())
}

func TestSessionRepo_GetByTokenHash(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	sessionRepo := NewSessionRepo(pool)

	u := createTestUser(t, userRepo)
	tokenHash := hashToken("lookup-token")

	s := &Session{
		UserID:    u.ID,
		TokenHash: tokenHash,
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent/2.0",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	ctx := context.Background()
	require.NoError(t, sessionRepo.Create(ctx, s))

	fetched, err := sessionRepo.GetByTokenHash(ctx, tokenHash)
	require.NoError(t, err)
	assert.Equal(t, s.ID, fetched.ID)
	assert.Equal(t, u.ID, fetched.UserID)
	assert.Equal(t, tokenHash, fetched.TokenHash)
	assert.Equal(t, "10.0.0.1", fetched.IPAddress)
}

func TestSessionRepo_GetByTokenHash_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	sessionRepo := NewSessionRepo(pool)

	ctx := context.Background()
	_, err := sessionRepo.GetByTokenHash(ctx, hashToken("nonexistent"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSessionRepo_TouchLastActive(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	sessionRepo := NewSessionRepo(pool)

	u := createTestUser(t, userRepo)
	s := &Session{
		UserID:    u.ID,
		TokenHash: hashToken("touch-token"),
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent/1.0",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	ctx := context.Background()
	require.NoError(t, sessionRepo.Create(ctx, s))

	newTime := time.Now().Add(5 * time.Minute).Truncate(time.Microsecond)
	err := sessionRepo.TouchLastActive(ctx, s.ID, newTime)
	require.NoError(t, err)

	fetched, err := sessionRepo.GetByTokenHash(ctx, s.TokenHash)
	require.NoError(t, err)
	assert.WithinDuration(t, newTime, fetched.LastActive, time.Millisecond)
}

func TestSessionRepo_Delete(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	sessionRepo := NewSessionRepo(pool)

	u := createTestUser(t, userRepo)
	s := &Session{
		UserID:    u.ID,
		TokenHash: hashToken("delete-token"),
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent/1.0",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	ctx := context.Background()
	require.NoError(t, sessionRepo.Create(ctx, s))

	err := sessionRepo.Delete(ctx, s.ID)
	require.NoError(t, err)

	_, err = sessionRepo.GetByTokenHash(ctx, s.TokenHash)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSessionRepo_Delete_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	sessionRepo := NewSessionRepo(pool)

	ctx := context.Background()
	err := sessionRepo.Delete(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSessionRepo_DeleteExpired(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	sessionRepo := NewSessionRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	// Create one expired and one valid session
	expired := &Session{
		UserID:    u.ID,
		TokenHash: hashToken("expired-token"),
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent/1.0",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}
	require.NoError(t, sessionRepo.Create(ctx, expired))

	valid := &Session{
		UserID:    u.ID,
		TokenHash: hashToken("valid-token"),
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent/1.0",
		ExpiresAt: time.Now().Add(1 * time.Hour), // still valid
	}
	require.NoError(t, sessionRepo.Create(ctx, valid))

	count, err := sessionRepo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Expired session is gone
	_, err = sessionRepo.GetByTokenHash(ctx, expired.TokenHash)
	assert.ErrorIs(t, err, ErrNotFound)

	// Valid session remains
	_, err = sessionRepo.GetByTokenHash(ctx, valid.TokenHash)
	require.NoError(t, err)
}

// hashToken is a test helper that hashes a token string the same way
// the auth service will — SHA-256 hex-encoded.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

