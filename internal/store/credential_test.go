package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	c := &BankCredential{
		BankCode:       "BBVA",
		AccountLabel:   "BBVA Main Account",
		CredentialsEnc: []byte("encrypted-creds"),
		CredentialsDEK: []byte("encrypted-dek"),
		CreatedBy:      u.ID,
		UpdatedBy:      u.ID,
	}

	err := credRepo.Create(ctx, c)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, c.ID)
	assert.Equal(t, 1, c.Version)
	assert.Equal(t, "active", c.Status)
	assert.Nil(t, c.DeletedAt)
	assert.False(t, c.CreatedAt.IsZero())
}

func TestCredentialRepo_GetByID(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	c := createTestCredential(t, credRepo, u.ID)

	ctx := context.Background()
	fetched, err := credRepo.GetByID(ctx, c.ID)
	require.NoError(t, err)

	assert.Equal(t, c.ID, fetched.ID)
	assert.Equal(t, c.BankCode, fetched.BankCode)
	assert.Equal(t, "Test Account", fetched.AccountLabel)
	assert.Equal(t, c.CredentialsEnc, fetched.CredentialsEnc)
	assert.Equal(t, 1, fetched.Version)
	assert.Equal(t, "active", fetched.Status)
}

func TestCredentialRepo_GetByID_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	credRepo := NewCredentialRepo(pool)

	ctx := context.Background()
	_, err := credRepo.GetByID(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCredentialRepo_List_ExcludesSoftDeleted(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	c1 := createTestCredential(t, credRepo, u.ID)
	createTestCredential(t, credRepo, u.ID) // c2

	// Soft-delete c1
	require.NoError(t, credRepo.SoftDelete(ctx, c1.ID, u.ID))

	creds, err := credRepo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, creds, 1) // only c2 remains
}

func TestCredentialRepo_Update_BumpsVersion(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	c := createTestCredential(t, credRepo, u.ID)
	assert.Equal(t, 1, c.Version)

	ctx := context.Background()
	c.AccountLabel = "Updated Label"
	c.CredentialsEnc = []byte("new-encrypted-creds")
	c.CredentialsDEK = []byte("new-encrypted-dek")
	c.UpdatedBy = u.ID

	err := credRepo.Update(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, 2, c.Version)

	// Second update
	c.AccountLabel = "Updated Again"
	err = credRepo.Update(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, 3, c.Version)

	// Verify via GetByID
	fetched, err := credRepo.GetByID(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, fetched.Version)
	assert.Equal(t, "Updated Again", fetched.AccountLabel)
}

func TestCredentialRepo_Update_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	credRepo := NewCredentialRepo(pool)

	ctx := context.Background()
	c := &BankCredential{ID: uuid.New()}
	err := credRepo.Update(ctx, c)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCredentialRepo_SoftDelete(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	c := createTestCredential(t, credRepo, u.ID)

	ctx := context.Background()
	err := credRepo.SoftDelete(ctx, c.ID, u.ID)
	require.NoError(t, err)

	// GetByID still works (returns the deleted record)
	fetched, err := credRepo.GetByID(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleted", fetched.Status)
	assert.NotNil(t, fetched.DeletedAt)

	// Double soft-delete should fail (already deleted)
	err = credRepo.SoftDelete(ctx, c.ID, u.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCredentialRepo_HardDeleteExpired(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	c1 := createTestCredential(t, credRepo, u.ID)
	c2 := createTestCredential(t, credRepo, u.ID)

	// Soft-delete both
	require.NoError(t, credRepo.SoftDelete(ctx, c1.ID, u.ID))
	require.NoError(t, credRepo.SoftDelete(ctx, c2.ID, u.ID))

	// Backdate c1's deleted_at to 100 days ago
	_, err := pool.Exec(ctx, "UPDATE bank_credentials SET deleted_at = now() - interval '100 days' WHERE id = $1", c1.ID)
	require.NoError(t, err)

	// Hard delete with 90 day retention — should only delete c1
	count, err := credRepo.HardDeleteExpired(ctx, 90)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// c1 is gone
	_, err = credRepo.GetByID(ctx, c1.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	// c2 still exists (deleted recently, within retention)
	fetched, err := credRepo.GetByID(ctx, c2.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleted", fetched.Status)
}

var testCredentialCounter int

// createTestCredential inserts a minimal credential for tests.
// Uses a unique bank code per call to avoid the unique active bank constraint.
func createTestCredential(t *testing.T, repo *CredentialRepo, userID uuid.UUID) *BankCredential {
	t.Helper()
	testCredentialCounter++
	bankCode := fmt.Sprintf("BANK%d", testCredentialCounter)

	c := &BankCredential{
		BankCode:       bankCode,
		AccountLabel:   "Test Account",
		CredentialsEnc: []byte("test-encrypted-creds"),
		CredentialsDEK: []byte("test-encrypted-dek"),
		CreatedBy:      userID,
		UpdatedBy:      userID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create test credential: %v", err)
	}
	return c
}
