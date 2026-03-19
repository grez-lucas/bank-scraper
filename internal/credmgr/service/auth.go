// Package service contains the business logic for the credential manager.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/crypto"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 30 * time.Minute
	pendingTokenTTL   = 5 * time.Minute
	bcryptCost        = 12
	tokenBytes        = 32 // 32 bytes → 64 hex chars
)

// Sentinel errors for the auth service.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrInvalidTOTP        = errors.New("invalid TOTP code")
	ErrSessionExpired     = errors.New("session expired")
)

// AuthService handles authentication: login, TOTP verification, and session management.
type AuthService struct {
	users    store.UserRepository
	sessions store.SessionRepository
	mk       crypto.MasterKey
	ttl      time.Duration
	logger   *slog.Logger

	mu      sync.Mutex
	pending map[string]*pendingLogin
}

type pendingLogin struct {
	userID    uuid.UUID
	ip        string
	ua        string
	expiresAt time.Time
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	users store.UserRepository,
	sessions store.SessionRepository,
	mk crypto.MasterKey,
	sessionTTL time.Duration,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		users:    users,
		sessions: sessions,
		mk:       mk,
		ttl:      sessionTTL,
		logger:   logger,
		pending:  make(map[string]*pendingLogin),
	}
}

// Login verifies the username and password. On success, returns a pending token
// that must be completed with VerifyTOTP. On failure, increments failed attempts
// and may lock the account.
func (s *AuthService) Login(ctx context.Context, username, password, ip, ua string) (bool, string, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Don't reveal whether user exists
			return false, "", fmt.Errorf("login: %w", ErrInvalidCredentials)
		}
		return false, "", fmt.Errorf("login: %w", err)
	}

	// Check lockout
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		s.logger.Warn("login rejected: account locked",
			slog.String("username", username),
			slog.String("ip", ip),
			slog.Time("locked_until", *user.LockedUntil))
		return false, "", fmt.Errorf("login: %w", ErrAccountLocked)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		count, incErr := s.users.IncrementFailedAttempts(ctx, user.ID)
		if incErr != nil {
			s.logger.Warn("failed to increment failed attempts", slog.Any("error", incErr))
		}
		s.logger.Warn("login failed: wrong password",
			slog.String("username", username),
			slog.String("ip", ip),
			slog.Int("failed_attempts", count))

		if count >= maxFailedAttempts {
			lockUntil := time.Now().Add(lockoutDuration)
			if lockErr := s.users.LockUntil(ctx, user.ID, lockUntil); lockErr != nil {
				s.logger.Warn("failed to lock account", slog.Any("error", lockErr))
			}
			s.logger.Warn("account locked after too many failed attempts",
				slog.String("username", username),
				slog.String("ip", ip),
				slog.Time("locked_until", lockUntil))
		}

		return false, "", fmt.Errorf("login: %w", ErrInvalidCredentials)
	}

	// Password correct — generate pending token for TOTP step
	token, err := generateToken()
	if err != nil {
		return false, "", fmt.Errorf("generate pending token: %w", err)
	}

	s.mu.Lock()
	s.sweepExpiredPendingLocked()
	s.pending[token] = &pendingLogin{
		userID:    user.ID,
		ip:        ip,
		ua:        ua,
		expiresAt: time.Now().Add(pendingTokenTTL),
	}
	s.mu.Unlock()

	s.logger.Info("login: password verified, TOTP required",
		slog.String("username", username))

	return true, token, nil
}

// VerifyTOTP validates the TOTP code for a pending login. On success, creates
// a session and returns the session token. Also resets failed login attempts.
func (s *AuthService) VerifyTOTP(ctx context.Context, pendingToken, code, ip, ua string) (string, error) {
	// Look up and consume the pending token
	s.mu.Lock()
	pl, ok := s.pending[pendingToken]
	if ok {
		delete(s.pending, pendingToken)
	}
	s.mu.Unlock()

	if !ok || pl.expiresAt.Before(time.Now()) {
		return "", fmt.Errorf("verify TOTP: %w", ErrInvalidCredentials)
	}

	// Get user to decrypt TOTP secret
	user, err := s.users.GetByID(ctx, pl.userID)
	if err != nil {
		return "", fmt.Errorf("verify TOTP: get user: %w", err)
	}

	// Decrypt TOTP secret
	secretBytes, err := crypto.Open(s.mk, user.TOTPSecretEnc, user.TOTPSecretDEK)
	if err != nil {
		return "", fmt.Errorf("verify TOTP: decrypt secret: %w", err)
	}

	// Validate TOTP code
	valid := totp.Validate(code, string(secretBytes))
	if !valid {
		s.logger.Warn("TOTP verification failed",
			slog.String("user_id", pl.userID.String()))
		return "", fmt.Errorf("verify TOTP: %w", ErrInvalidTOTP)
	}

	// TOTP valid — create session
	sessionToken, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	session := &store.Session{
		UserID:    pl.userID,
		TokenHash: hashTokenStr(sessionToken),
		IPAddress: ip,
		UserAgent: ua,
		ExpiresAt: time.Now().Add(s.ttl),
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	// Reset failed attempts on successful login
	if err := s.users.ResetFailedAttempts(ctx, pl.userID); err != nil {
		s.logger.Warn("failed to reset failed attempts", slog.Any("error", err))
	}

	s.logger.Info("login complete",
		slog.String("user_id", pl.userID.String()),
		slog.String("session_id", session.ID.String()))

	return sessionToken, nil
}

// ValidateSession checks if a session token is valid and the session hasn't
// expired due to inactivity. Returns the authenticated user on success.
// Also touches the session's last_active timestamp (sliding expiry).
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*store.User, error) {
	hash := hashTokenStr(token)

	session, err := s.sessions.GetByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("validate session: %w", ErrSessionExpired)
		}
		return nil, fmt.Errorf("validate session: %w", err)
	}

	// Check inactivity timeout
	if time.Since(session.LastActive) > s.ttl {
		// Clean up expired session
		if err := s.sessions.Delete(ctx, session.ID); err != nil {
			s.logger.Warn("failed to delete expired session", slog.Any("error", err))
		}
		return nil, fmt.Errorf("validate session: inactive for %s: %w",
			time.Since(session.LastActive).Truncate(time.Second), ErrSessionExpired)
	}

	// Touch last_active (sliding expiry)
	if err := s.sessions.TouchLastActive(ctx, session.ID, time.Now()); err != nil {
		s.logger.Warn("failed to touch session", slog.Any("error", err))
	}

	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("validate session: get user: %w", err)
	}

	return user, nil
}

// Logout destroys the session associated with the given token.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	hash := hashTokenStr(token)

	session, err := s.sessions.GetByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // already gone
		}
		return fmt.Errorf("logout: %w", err)
	}

	if err := s.sessions.Delete(ctx, session.ID); err != nil {
		return fmt.Errorf("logout: %w", err)
	}

	s.logger.Info("session destroyed",
		slog.String("session_id", session.ID.String()),
		slog.String("user_id", session.UserID.String()))

	return nil
}

// sweepExpiredPendingLocked removes expired pending login entries.
// Must be called with s.mu held.
func (s *AuthService) sweepExpiredPendingLocked() {
	now := time.Now()
	for token, pl := range s.pending {
		if pl.expiresAt.Before(now) {
			delete(s.pending, token)
		}
	}
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// generateToken creates a cryptographically random hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashTokenStr returns the SHA-256 hex digest of a token string.
func hashTokenStr(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
