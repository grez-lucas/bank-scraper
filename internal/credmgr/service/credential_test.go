package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/aynifx/bank-scraper/internal/credmgr/crypto"
	"github.com/aynifx/bank-scraper/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fake credential repo ---

type fakeCredentialRepo struct {
	mu    sync.Mutex
	creds map[uuid.UUID]*store.BankCredential
}

func newFakeCredentialRepo() *fakeCredentialRepo {
	return &fakeCredentialRepo{creds: make(map[uuid.UUID]*store.BankCredential)}
}

func (r *fakeCredentialRepo) Create(_ context.Context, c *store.BankCredential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.ID = uuid.New()
	c.Version = 1
	c.Status = store.CredentialStatusActive
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	clone := *c
	r.creds[c.ID] = &clone
	return nil
}

func (r *fakeCredentialRepo) GetByID(_ context.Context, id uuid.UUID) (*store.BankCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.creds[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *c
	return &clone, nil
}

func (r *fakeCredentialRepo) List(_ context.Context) ([]store.BankCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []store.BankCredential
	for _, c := range r.creds {
		if c.Status == store.CredentialStatusActive {
			result = append(result, *c)
		}
	}
	return result, nil
}

func (r *fakeCredentialRepo) Update(_ context.Context, c *store.BankCredential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.creds[c.ID]
	if !ok || existing.Status != store.CredentialStatusActive {
		return store.ErrNotFound
	}
	c.Version = existing.Version + 1
	c.UpdatedAt = time.Now()
	clone := *c
	r.creds[c.ID] = &clone
	return nil
}

func (r *fakeCredentialRepo) SoftDelete(_ context.Context, id, deletedBy uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.creds[id]
	if !ok || c.Status != store.CredentialStatusActive {
		return store.ErrNotFound
	}
	now := time.Now()
	c.Status = store.CredentialStatusDeleted
	c.DeletedAt = &now
	c.UpdatedBy = deletedBy
	return nil
}

func (r *fakeCredentialRepo) GetActiveByBankCode(_ context.Context, bankCode string) (*store.BankCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.creds {
		if c.BankCode == bankCode && c.Status == store.CredentialStatusActive {
			clone := *c
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeCredentialRepo) HardDeleteExpired(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

// --- fake audit repo ---

type fakeAuditRepo struct {
	mu   sync.Mutex
	logs []store.AuditLog
}

func newFakeAuditRepo() *fakeAuditRepo {
	return &fakeAuditRepo{}
}

func (r *fakeAuditRepo) Create(_ context.Context, l *store.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	l.ID = int64(len(r.logs) + 1)
	l.Timestamp = time.Now()
	r.logs = append(r.logs, *l)
	return nil
}

func (r *fakeAuditRepo) List(_ context.Context, _ store.AuditFilter) ([]store.AuditLog, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.logs, int64(len(r.logs)), nil
}

func (r *fakeAuditRepo) lastLog() *store.AuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.logs) == 0 {
		return nil
	}
	l := r.logs[len(r.logs)-1]
	return &l
}

// --- fake credential tester ---

type fakeTester struct {
	err error
}

func (t *fakeTester) TestCredentials(_ context.Context, _ string, _ map[string]string) error {
	return t.err
}

// --- test helpers ---

func newTestCredentialService(
	credRepo store.CredentialRepository,
	auditRepo *fakeAuditRepo,
	tester CredentialTester,
) *CredentialService {
	mk, _ := crypto.ParseMasterKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	aw := NewAuditWriter(auditRepo, slog.Default())
	return NewCredentialService(credRepo, aw, mk, tester, slog.Default())
}

// --- tests ---

func TestCredentialService_Create(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()
	cred := PlaintextCredential{
		BankCode: "BBVA",
		Label:    "BBVA Main",
		Fields:   map[string]string{"company_code": "123", "user_code": "admin", "password": "secret"},
	}

	id, err := svc.Create(ctx, cred, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)

	// Verify stored credential is encrypted (not plaintext)
	stored, err := credRepo.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "BBVA", stored.BankCode)
	assert.Equal(t, "BBVA Main", stored.AccountLabel)
	assert.NotEmpty(t, stored.CredentialsEnc)
	assert.NotEmpty(t, stored.CredentialsDEK)
	// Encrypted data should NOT contain plaintext
	assert.NotContains(t, string(stored.CredentialsEnc), "secret")

	// Verify audit log
	al := auditRepo.lastLog()
	require.NotNil(t, al)
	assert.Equal(t, "credential_created", al.Action)
	assert.Equal(t, "credential", al.TargetType)
	assert.Equal(t, id.String(), al.TargetID)
	assert.True(t, al.Success)
}

func TestCredentialService_Create_CanDecrypt(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	mk, _ := crypto.ParseMasterKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	aw := NewAuditWriter(auditRepo, slog.Default())
	svc := NewCredentialService(credRepo, aw, mk, &fakeTester{}, slog.Default())

	ctx := context.Background()
	cred := PlaintextCredential{
		BankCode: "BBVA",
		Label:    "Test",
		Fields:   map[string]string{"password": "mysecret"},
	}
	id, err := svc.Create(ctx, cred, uuid.New(), "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Decrypt and verify
	stored, _ := credRepo.GetByID(ctx, id)
	plaintext, err := crypto.Open(mk, stored.CredentialsEnc, stored.CredentialsDEK)
	require.NoError(t, err)

	var fields map[string]string
	require.NoError(t, json.Unmarshal(plaintext, &fields))
	assert.Equal(t, "mysecret", fields["password"])
}

func TestCredentialService_List(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	_, err := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "Account 1", Fields: map[string]string{"k": "v"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	_, err = svc.Create(ctx, PlaintextCredential{BankCode: "BCP", Label: "Account 2", Fields: map[string]string{"k": "v"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	summaries, err := svc.List(ctx, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.Len(t, summaries, 2)

	// Summaries should NOT contain encrypted data
	for _, s := range summaries {
		assert.NotEmpty(t, s.BankCode)
		assert.NotEmpty(t, s.Label)
		assert.Equal(t, 1, s.Version)
		assert.Equal(t, store.CredentialStatusActive, s.Status)
	}

	// Audit log for list access
	al := auditRepo.lastLog()
	require.NotNil(t, al)
	assert.Equal(t, "credentials_listed", al.Action)
}

func TestCredentialService_Update(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()
	id, _ := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "Original", Fields: map[string]string{"password": "old"}}, userID, "10.0.0.1", "TestAgent")

	err := svc.Update(ctx, id, PlaintextCredential{BankCode: "BBVA", Label: "Updated", Fields: map[string]string{"password": "new"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Verify version bumped
	stored, _ := credRepo.GetByID(ctx, id)
	assert.Equal(t, 2, stored.Version)
	assert.Equal(t, "Updated", stored.AccountLabel)

	// Audit log
	al := auditRepo.lastLog()
	assert.Equal(t, "credential_updated", al.Action)
	assert.Equal(t, id.String(), al.TargetID)
}

func TestCredentialService_SoftDelete(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()
	id, _ := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "ToDelete", Fields: map[string]string{"k": "v"}}, userID, "10.0.0.1", "TestAgent")

	err := svc.SoftDelete(ctx, id, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Should not appear in list
	summaries, _ := svc.List(ctx, userID, "10.0.0.1", "TestAgent")
	assert.Empty(t, summaries)

	// Audit log
	al := auditRepo.lastLog()
	// Last log is from List, the one before is from SoftDelete
	logs, _, _ := auditRepo.List(ctx, store.AuditFilter{})
	var deleteLog *store.AuditLog
	for i := range logs {
		if logs[i].Action == "credential_deleted" {
			deleteLog = &logs[i]
			break
		}
	}
	require.NotNil(t, deleteLog)
	assert.Equal(t, id.String(), deleteLog.TargetID)
	_ = al // suppress unused
}

func TestCredentialService_Test_Valid(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	tester := &fakeTester{err: nil}
	svc := newTestCredentialService(credRepo, auditRepo, tester)

	ctx := context.Background()
	err := svc.Test(ctx, PlaintextCredential{
		BankCode: "BBVA",
		Fields:   map[string]string{"company_code": "123", "user_code": "admin", "password": "valid"},
	})
	require.NoError(t, err)
}

func TestCredentialService_Test_Invalid(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	tester := &fakeTester{err: fmt.Errorf("login: %w", ErrInvalidCredentials)}
	svc := newTestCredentialService(credRepo, auditRepo, tester)

	ctx := context.Background()
	err := svc.Test(ctx, PlaintextCredential{
		BankCode: "BBVA",
		Fields:   map[string]string{"company_code": "123", "user_code": "admin", "password": "wrong"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestCredentialService_SoftDelete_NotFound(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	err := svc.SoftDelete(ctx, uuid.New(), uuid.New(), "10.0.0.1", "TestAgent")
	require.Error(t, err)
}

// --- One credential per bank ---

func TestCredentialService_Create_DuplicateBank(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	// First BBVA credential — should succeed
	_, err := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "First", Fields: map[string]string{"password": "s"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Second BBVA credential — should fail with ErrDuplicateBank
	_, err = svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "Second", Fields: map[string]string{"password": "s"}}, userID, "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateBank)
}

func TestCredentialService_Create_DifferentBanks(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	_, err := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "BBVA Acc", Fields: map[string]string{"p": "1"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	_, err = svc.Create(ctx, PlaintextCredential{BankCode: "BCP", Label: "BCP Acc", Fields: map[string]string{"p": "2"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
}

func TestCredentialService_Create_AfterDeleteAllowed(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	// Create and delete
	id, err := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "Old", Fields: map[string]string{"p": "1"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	require.NoError(t, svc.SoftDelete(ctx, id, userID, "10.0.0.1", "TestAgent"))

	// New BBVA credential should succeed (old one is soft-deleted)
	_, err = svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "New", Fields: map[string]string{"p": "2"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
}

// --- Scraper reads credentials from DB ---

func TestCredentialService_GetCredentials_Found(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	_, err := svc.Create(ctx, PlaintextCredential{
		BankCode: "BBVA",
		Label:    "BBVA Main",
		Fields:   map[string]string{"company_code": "123", "user_code": "admin", "password": "secret"},
	}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	fields, err := svc.GetCredentials(ctx, "BBVA")
	require.NoError(t, err)
	assert.Equal(t, "123", fields["company_code"])
	assert.Equal(t, "admin", fields["user_code"])
	assert.Equal(t, "secret", fields["password"])
}

func TestCredentialService_GetCredentials_NotConfigured(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	_, err := svc.GetCredentials(ctx, "BBVA")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCredentialNotConfigured)
}

func TestCredentialService_GetCredentials_WrongBank(t *testing.T) {
	credRepo := newFakeCredentialRepo()
	auditRepo := newFakeAuditRepo()
	svc := newTestCredentialService(credRepo, auditRepo, &fakeTester{})

	ctx := context.Background()
	userID := uuid.New()

	// Only BBVA is configured
	_, err := svc.Create(ctx, PlaintextCredential{BankCode: "BBVA", Label: "BBVA", Fields: map[string]string{"p": "1"}}, userID, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// BCP is not configured
	_, err = svc.GetCredentials(ctx, "BCP")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCredentialNotConfigured)
}
