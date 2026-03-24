package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)

	ctx := context.Background()
	a := &Account{
		BankCode:      "BBVA",
		AccountNumber: "001-12345678-0-01",
		Currency:      "PEN",
		AccountType:   "checking",
		CredentialID:  cred.ID,
	}

	err := accountRepo.Create(ctx, a)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, a.ID)
	assert.Equal(t, "active", a.Status)
	assert.Nil(t, a.LastSyncedAt)
	assert.False(t, a.CreatedAt.IsZero())
	assert.False(t, a.UpdatedAt.IsZero())
}

func TestAccountRepo_Create_DuplicateAccountNumber(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)

	ctx := context.Background()
	a1 := &Account{
		BankCode:      "BBVA",
		AccountNumber: "001-12345678-0-01",
		Currency:      "PEN",
		CredentialID:  cred.ID,
	}
	require.NoError(t, accountRepo.Create(ctx, a1))

	a2 := &Account{
		BankCode:      "BBVA",
		AccountNumber: "001-12345678-0-01",
		Currency:      "PEN",
		CredentialID:  cred.ID,
	}
	err := accountRepo.Create(ctx, a2)
	require.Error(t, err, "duplicate (bank_code, account_number) should fail")
}

func TestAccountRepo_GetByID(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)
	a := createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-11111111-0-01", "PEN")

	ctx := context.Background()
	fetched, err := accountRepo.GetByID(ctx, a.ID)
	require.NoError(t, err)

	assert.Equal(t, a.ID, fetched.ID)
	assert.Equal(t, "BBVA", fetched.BankCode)
	assert.Equal(t, "001-11111111-0-01", fetched.AccountNumber)
	assert.Equal(t, "PEN", fetched.Currency)
	assert.Equal(t, "checking", fetched.AccountType)
	assert.Equal(t, "active", fetched.Status)
	assert.Equal(t, cred.ID, fetched.CredentialID)
}

func TestAccountRepo_GetByID_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	accountRepo := NewAccountRepo(pool)

	ctx := context.Background()
	_, err := accountRepo.GetByID(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAccountRepo_List(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)
	createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-11111111-0-01", "PEN")
	createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-22222222-0-01", "USD")

	ctx := context.Background()
	accounts, err := accountRepo.List(ctx, AccountFilter{})
	require.NoError(t, err)
	assert.Len(t, accounts, 2)
}

func TestAccountRepo_List_FilterByBankCode(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred1 := createTestCredential(t, credRepo, u.ID) // unique bank code per call
	cred2 := createTestCredential(t, credRepo, u.ID)
	createTestAccount(t, accountRepo, cred1.ID, "BBVA", "001-11111111-0-01", "PEN")
	createTestAccount(t, accountRepo, cred2.ID, "INTERBANK", "002-22222222-0-01", "PEN")

	ctx := context.Background()
	bankCode := "BBVA"
	accounts, err := accountRepo.List(ctx, AccountFilter{BankCode: &bankCode})
	require.NoError(t, err)
	assert.Len(t, accounts, 1)
	assert.Equal(t, "BBVA", accounts[0].BankCode)
}

func TestAccountRepo_List_FilterByCurrency(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)
	createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-11111111-0-01", "PEN")
	createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-22222222-0-01", "USD")

	ctx := context.Background()
	currency := "USD"
	accounts, err := accountRepo.List(ctx, AccountFilter{Currency: &currency})
	require.NoError(t, err)
	assert.Len(t, accounts, 1)
	assert.Equal(t, "USD", accounts[0].Currency)
}

func TestAccountRepo_UpsertBatch(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)

	ctx := context.Background()

	// First upsert: insert two new accounts
	batch := []Account{
		{BankCode: "BBVA", AccountNumber: "001-11111111-0-01", Currency: "PEN", AccountType: "checking"},
		{BankCode: "BBVA", AccountNumber: "001-22222222-0-01", Currency: "USD", AccountType: "savings"},
	}

	err := accountRepo.UpsertBatch(ctx, cred.ID, batch)
	require.NoError(t, err)

	accounts, err := accountRepo.List(ctx, AccountFilter{})
	require.NoError(t, err)
	assert.Len(t, accounts, 2)

	// Second upsert: update existing + add new
	batch2 := []Account{
		{BankCode: "BBVA", AccountNumber: "001-11111111-0-01", Currency: "PEN", AccountType: "checking"},
		{BankCode: "BBVA", AccountNumber: "001-33333333-0-01", Currency: "PEN", AccountType: "checking"},
	}

	err = accountRepo.UpsertBatch(ctx, cred.ID, batch2)
	require.NoError(t, err)

	accounts, err = accountRepo.List(ctx, AccountFilter{})
	require.NoError(t, err)
	assert.Len(t, accounts, 3)
}

func TestAccountRepo_UpdateLastSynced(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	credRepo := NewCredentialRepo(pool)
	accountRepo := NewAccountRepo(pool)

	u := createTestUser(t, userRepo)
	cred := createTestCredential(t, credRepo, u.ID)
	a := createTestAccount(t, accountRepo, cred.ID, "BBVA", "001-11111111-0-01", "PEN")
	assert.Nil(t, a.LastSyncedAt)

	ctx := context.Background()
	err := accountRepo.UpdateLastSynced(ctx, a.ID)
	require.NoError(t, err)

	fetched, err := accountRepo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	assert.NotNil(t, fetched.LastSyncedAt)
	assert.WithinDuration(t, time.Now(), *fetched.LastSyncedAt, 5*time.Second)
}

func TestAccountRepo_UpdateLastSynced_NotFound(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	accountRepo := NewAccountRepo(pool)

	ctx := context.Background()
	err := accountRepo.UpdateLastSynced(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

// createTestAccount inserts a minimal account for tests.
func createTestAccount(t *testing.T, repo *AccountRepo, credentialID uuid.UUID, bankCode, accountNumber, currency string) *Account {
	t.Helper()

	a := &Account{
		BankCode:      bankCode,
		AccountNumber: accountNumber,
		Currency:      currency,
		AccountType:   "checking",
		CredentialID:  credentialID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("create test account: %v", err)
	}
	return a
}
