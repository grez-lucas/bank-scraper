package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/crypto"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// --- fake repos ---

type fakeUserRepo struct {
	mu    sync.Mutex
	users map[uuid.UUID]*store.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[uuid.UUID]*store.User)}
}

func (r *fakeUserRepo) Create(_ context.Context, u *store.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if existing.Username == u.Username {
			return fmt.Errorf("duplicate username")
		}
	}
	u.ID = uuid.New()
	u.IsActive = true
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	clone := *u
	r.users[u.ID] = &clone
	return nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*store.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *u
	return &clone, nil
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*store.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Username == username {
			clone := *u
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeUserRepo) IncrementFailedAttempts(_ context.Context, id uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return 0, store.ErrNotFound
	}
	u.FailedAttempts++
	return u.FailedAttempts, nil
}

func (r *fakeUserRepo) ResetFailedAttempts(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return store.ErrNotFound
	}
	u.FailedAttempts = 0
	u.LockedUntil = nil
	return nil
}

func (r *fakeUserRepo) LockUntil(_ context.Context, id uuid.UUID, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return store.ErrNotFound
	}
	u.LockedUntil = &until
	return nil
}

type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*store.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[uuid.UUID]*store.Session)}
}

func (r *fakeSessionRepo) Create(_ context.Context, s *store.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s.ID = uuid.New()
	s.LastActive = time.Now()
	s.CreatedAt = time.Now()
	clone := *s
	r.sessions[s.ID] = &clone
	return nil
}

func (r *fakeSessionRepo) GetByTokenHash(_ context.Context, hash string) (*store.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.sessions {
		if s.TokenHash == hash {
			clone := *s
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeSessionRepo) TouchLastActive(_ context.Context, id uuid.UUID, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return store.ErrNotFound
	}
	s.LastActive = now
	return nil
}

func (r *fakeSessionRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return store.ErrNotFound
	}
	delete(r.sessions, id)
	return nil
}

func (r *fakeSessionRepo) DeleteExpired(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for id, s := range r.sessions {
		if s.ExpiresAt.Before(time.Now()) {
			delete(r.sessions, id)
			count++
		}
	}
	return count, nil
}

// --- test helpers ---

func testMasterKey(t *testing.T) crypto.MasterKey {
	t.Helper()
	mk, err := crypto.ParseMasterKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return mk
}

// createTestUserWithTOTP creates a user with a known password and TOTP secret
// in the fake repo. Returns the user and the raw TOTP secret for generating codes.
func createTestUserWithTOTP(t *testing.T, userRepo *fakeUserRepo, mk crypto.MasterKey) (*store.User, string) {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	require.NoError(t, err)

	// Generate a real TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "BankScraper",
		AccountName: "testuser",
	})
	require.NoError(t, err)
	totpSecret := key.Secret()

	encSecret, encDEK, err := crypto.Seal(mk, []byte(totpSecret))
	require.NoError(t, err)

	u := &store.User{
		Username:      "testuser",
		PasswordHash:  string(passwordHash),
		TOTPSecretEnc: encSecret,
		TOTPSecretDEK: encDEK,
	}

	ctx := context.Background()
	require.NoError(t, userRepo.Create(ctx, u))
	return u, totpSecret
}

func newTestAuthService(userRepo store.UserRepository, sessionRepo store.SessionRepository, mk crypto.MasterKey) *AuthService {
	aw := NewAuditWriter(newFakeAuditRepo(), slog.Default())
	return NewAuthService(userRepo, sessionRepo, aw, mk, 15*time.Minute, slog.Default())
}

func newTestAuthServiceWithAudit(userRepo store.UserRepository, sessionRepo store.SessionRepository, mk crypto.MasterKey) (*AuthService, *fakeAuditRepo) {
	ar := newFakeAuditRepo()
	aw := NewAuditWriter(ar, slog.Default())
	return NewAuthService(userRepo, sessionRepo, aw, mk, 15*time.Minute, slog.Default()), ar
}

// --- tests ---

func TestLogin_CorrectPassword(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	totpRequired, pendingToken, err := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.True(t, totpRequired)
	assert.NotEmpty(t, pendingToken)
}

func TestLogin_WrongPassword(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	user, _ := createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, _, err := svc.Login(ctx, "testuser", "wrong-password", "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))

	// Failed attempts should be incremented
	u, _ := userRepo.GetByID(ctx, user.ID)
	assert.Equal(t, 1, u.FailedAttempts)
}

func TestLogin_AccountLockout(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	createTestUserWithTOTP(t, userRepo, mk)
	ctx := context.Background()

	// Fail 5 times
	for i := 0; i < 5; i++ {
		_, _, err := svc.Login(ctx, "testuser", "wrong-password", "10.0.0.1", "TestAgent")
		require.Error(t, err)
	}

	// 6th attempt with correct password should still fail (locked)
	_, _, err := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAccountLocked))
}

func TestLogin_UserNotFound(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc, auditRepo := newTestAuthServiceWithAudit(userRepo, sessionRepo, mk)

	ctx := context.Background()
	_, _, err := svc.Login(ctx, "nonexistent", "password", "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))

	// Verify audit log entry for unknown user
	last := auditRepo.lastLog()
	require.NotNil(t, last)
	assert.Equal(t, AuditLoginFailed, last.Action)
	assert.Nil(t, last.UserID)
	assert.Equal(t, "nonexistent", last.TargetID)
	assert.Equal(t, "10.0.0.1", last.IPAddress)
	assert.False(t, last.Success)
	assert.Equal(t, "unknown_user", last.Details["reason"])
}

func TestVerifyTOTP_ValidCode(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	_, totpSecret := createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, pendingToken, err := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Generate a valid TOTP code
	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)

	sessionToken, err := svc.VerifyTOTP(ctx, pendingToken, code, "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.NotEmpty(t, sessionToken)
	assert.Len(t, sessionToken, 64) // 32 bytes hex-encoded
}

func TestVerifyTOTP_InvalidCode(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, pendingToken, err := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	_, err = svc.VerifyTOTP(ctx, pendingToken, "000000", "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTOTP))
}

func TestVerifyTOTP_InvalidPendingToken(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	ctx := context.Background()
	_, err := svc.VerifyTOTP(ctx, "nonexistent-token", "123456", "10.0.0.1", "TestAgent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestValidateSession_Valid(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	_, totpSecret := createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, pendingToken, _ := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	code, _ := totp.GenerateCode(totpSecret, time.Now())
	sessionToken, _ := svc.VerifyTOTP(ctx, pendingToken, code, "10.0.0.1", "TestAgent")

	user, err := svc.ValidateSession(ctx, sessionToken)
	require.NoError(t, err)
	assert.Equal(t, "testuser", user.Username)
}

func TestValidateSession_Expired(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	// Use a very short TTL so the session expires immediately
	aw := NewAuditWriter(newFakeAuditRepo(), slog.Default())
	svc := NewAuthService(userRepo, sessionRepo, aw, mk, 1*time.Millisecond, slog.Default())

	_, totpSecret := createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, pendingToken, _ := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	code, _ := totp.GenerateCode(totpSecret, time.Now())
	sessionToken, _ := svc.VerifyTOTP(ctx, pendingToken, code, "10.0.0.1", "TestAgent")

	// Wait for session to expire
	time.Sleep(5 * time.Millisecond)

	_, err := svc.ValidateSession(ctx, sessionToken)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSessionExpired))
}

func TestLogout(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	_, totpSecret := createTestUserWithTOTP(t, userRepo, mk)

	ctx := context.Background()
	_, pendingToken, _ := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	code, _ := totp.GenerateCode(totpSecret, time.Now())
	sessionToken, _ := svc.VerifyTOTP(ctx, pendingToken, code, "10.0.0.1", "TestAgent")

	err := svc.Logout(ctx, sessionToken)
	require.NoError(t, err)

	// Session should no longer be valid
	_, err = svc.ValidateSession(ctx, sessionToken)
	require.Error(t, err)
}

func TestLogin_ResetsFailedAttemptsOnSuccess(t *testing.T) {
	userRepo := newFakeUserRepo()
	sessionRepo := newFakeSessionRepo()
	mk := testMasterKey(t)
	svc := newTestAuthService(userRepo, sessionRepo, mk)

	user, totpSecret := createTestUserWithTOTP(t, userRepo, mk)
	ctx := context.Background()

	// Fail twice
	_, _, _ = svc.Login(ctx, "testuser", "wrong-password", "10.0.0.1", "TestAgent")
	_, _, _ = svc.Login(ctx, "testuser", "wrong-password", "10.0.0.1", "TestAgent")

	// Succeed + complete TOTP
	_, pendingToken, err := svc.Login(ctx, "testuser", "correct-password", "10.0.0.1", "TestAgent")
	require.NoError(t, err)
	code, _ := totp.GenerateCode(totpSecret, time.Now())
	_, err = svc.VerifyTOTP(ctx, pendingToken, code, "10.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Failed attempts should be reset
	u, _ := userRepo.GetByID(ctx, user.ID)
	assert.Equal(t, 0, u.FailedAttempts)
}

// hashToken mirrors the auth service's token hashing.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
