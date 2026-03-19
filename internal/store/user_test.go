package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	u := &User{
		Username:      "admin",
		PasswordHash:  "$2a$12$testhash",
		TOTPSecretEnc: []byte("enc-secret"),
		TOTPSecretDEK: []byte("enc-dek"),
	}

	ctx := context.Background()
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.True(t, u.IsActive)
	assert.Equal(t, 0, u.FailedAttempts)
	assert.Nil(t, u.LockedUntil)
	assert.False(t, u.CreatedAt.IsZero())
	assert.False(t, u.UpdatedAt.IsZero())
}

func TestUserRepo_Create_DuplicateUsername(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	u := &User{
		Username:      "admin",
		PasswordHash:  "$2a$12$testhash",
		TOTPSecretEnc: []byte("enc"),
		TOTPSecretDEK: []byte("dek"),
	}

	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, u))

	u2 := &User{
		Username:      "admin",
		PasswordHash:  "$2a$12$otherhash",
		TOTPSecretEnc: []byte("enc2"),
		TOTPSecretDEK: []byte("dek2"),
	}
	err := repo.Create(ctx, u2)
	require.Error(t, err, "duplicate username should fail")
}

func TestUserRepo_GetByID(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	created := createTestUser(t, repo)

	ctx := context.Background()
	fetched, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Username, fetched.Username)
	assert.Equal(t, created.PasswordHash, fetched.PasswordHash)
	assert.Equal(t, created.TOTPSecretEnc, fetched.TOTPSecretEnc)
	assert.Equal(t, created.TOTPSecretDEK, fetched.TOTPSecretDEK)
}

func TestUserRepo_GetByID_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepo_GetByUsername(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	created := createTestUser(t, repo)

	ctx := context.Background()
	fetched, err := repo.GetByUsername(ctx, created.Username)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
}

func TestUserRepo_GetByUsername_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	ctx := context.Background()
	_, err := repo.GetByUsername(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepo_IncrementFailedAttempts(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	u := createTestUser(t, repo)
	ctx := context.Background()

	count, err := repo.IncrementFailedAttempts(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = repo.IncrementFailedAttempts(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify via GetByID
	fetched, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, fetched.FailedAttempts)
}

func TestUserRepo_LockUntil(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	u := createTestUser(t, repo)
	ctx := context.Background()

	lockTime := time.Now().Add(30 * time.Minute).Truncate(time.Microsecond)
	err := repo.LockUntil(ctx, u.ID, lockTime)
	require.NoError(t, err)

	fetched, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched.LockedUntil)
	assert.WithinDuration(t, lockTime, *fetched.LockedUntil, time.Millisecond)
}

func TestUserRepo_ResetFailedAttempts(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	u := createTestUser(t, repo)
	ctx := context.Background()

	// Increment + lock
	_, _ = repo.IncrementFailedAttempts(ctx, u.ID)
	_, _ = repo.IncrementFailedAttempts(ctx, u.ID)
	_ = repo.LockUntil(ctx, u.ID, time.Now().Add(30*time.Minute))

	// Reset
	err := repo.ResetFailedAttempts(ctx, u.ID)
	require.NoError(t, err)

	fetched, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, fetched.FailedAttempts)
	assert.Nil(t, fetched.LockedUntil)
}

func TestUserRepo_ResetFailedAttempts_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	repo := NewUserRepo(pool)

	ctx := context.Background()
	err := repo.ResetFailedAttempts(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}
