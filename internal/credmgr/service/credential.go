package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/crypto"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// Audit action constants for credential operations.
const (
	AuditCredentialCreated = "credential_created"
	AuditCredentialUpdated = "credential_updated"
	AuditCredentialDeleted = "credential_deleted"
	AuditCredentialsListed = "credentials_listed"
)

// PlaintextCredential represents bank credentials before encryption.
type PlaintextCredential struct {
	BankCode string
	Label    string
	Fields   map[string]string // Bank-specific: company_code, user_code, password, etc.
}

// CredentialSummary is the safe view returned to the UI (no secrets).
type CredentialSummary struct {
	ID        uuid.UUID
	BankCode  string
	Label     string
	Version   int
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CredentialTester validates bank credentials by attempting a login.
type CredentialTester interface {
	TestCredentials(ctx context.Context, bankCode string, fields map[string]string) error
}

// CredentialService manages the bank credential lifecycle.
type CredentialService struct {
	creds  store.CredentialRepository
	audit  store.AuditLogRepository
	mk     crypto.MasterKey
	tester CredentialTester
	logger *slog.Logger
}

// NewCredentialService creates a new CredentialService.
func NewCredentialService(
	creds store.CredentialRepository,
	audit store.AuditLogRepository,
	mk crypto.MasterKey,
	tester CredentialTester,
	logger *slog.Logger,
) *CredentialService {
	return &CredentialService{
		creds:  creds,
		audit:  audit,
		mk:     mk,
		tester: tester,
		logger: logger,
	}
}

// Create encrypts and stores a new bank credential. Returns the new credential ID.
func (s *CredentialService) Create(ctx context.Context, cred PlaintextCredential, userID uuid.UUID, ip, ua string) (uuid.UUID, error) {
	encData, encDEK, err := s.encryptFields(cred.Fields)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encrypt credential: %w", err)
	}

	c := &store.BankCredential{
		BankCode:       cred.BankCode,
		AccountLabel:   cred.Label,
		CredentialsEnc: encData,
		CredentialsDEK: encDEK,
		CreatedBy:      userID,
		UpdatedBy:      userID,
	}

	if err := s.creds.Create(ctx, c); err != nil {
		return uuid.Nil, fmt.Errorf("create credential: %w", err)
	}

	s.auditLog(ctx, &userID, AuditCredentialCreated, "credential", c.ID.String(), ip, ua, true, nil)

	s.logger.Info("credential created",
		slog.String("credential_id", c.ID.String()),
		slog.String("bank_code", cred.BankCode))

	return c.ID, nil
}

// List returns credential summaries (no encrypted data) for all active credentials.
func (s *CredentialService) List(ctx context.Context, userID uuid.UUID, ip, ua string) ([]CredentialSummary, error) {
	creds, err := s.creds.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}

	summaries := make([]CredentialSummary, len(creds))
	for i, c := range creds {
		summaries[i] = CredentialSummary{
			ID:        c.ID,
			BankCode:  c.BankCode,
			Label:     c.AccountLabel,
			Version:   c.Version,
			Status:    c.Status,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		}
	}

	s.auditLog(ctx, &userID, AuditCredentialsListed, "credential", "", ip, ua, true, nil)

	return summaries, nil
}

// Update re-encrypts and updates an existing credential.
func (s *CredentialService) Update(ctx context.Context, id uuid.UUID, cred PlaintextCredential, userID uuid.UUID, ip, ua string) error {
	encData, encDEK, err := s.encryptFields(cred.Fields)
	if err != nil {
		return fmt.Errorf("update credential: %w", err)
	}

	c := &store.BankCredential{
		ID:             id,
		BankCode:       cred.BankCode,
		AccountLabel:   cred.Label,
		CredentialsEnc: encData,
		CredentialsDEK: encDEK,
		UpdatedBy:      userID,
	}

	if err := s.creds.Update(ctx, c); err != nil {
		s.auditLog(ctx, &userID, AuditCredentialUpdated, "credential", id.String(), ip, ua, false,
			map[string]any{"error": err.Error()})
		return fmt.Errorf("update credential: %w", err)
	}

	s.auditLog(ctx, &userID, AuditCredentialUpdated, "credential", id.String(), ip, ua, true,
		map[string]any{"new_version": c.Version})

	s.logger.Info("credential updated",
		slog.String("credential_id", id.String()),
		slog.Int("version", c.Version))

	return nil
}

// SoftDelete marks a credential as deleted.
func (s *CredentialService) SoftDelete(ctx context.Context, id uuid.UUID, userID uuid.UUID, ip, ua string) error {
	if err := s.creds.SoftDelete(ctx, id, userID); err != nil {
		s.auditLog(ctx, &userID, AuditCredentialDeleted, "credential", id.String(), ip, ua, false,
			map[string]any{"error": err.Error()})
		return fmt.Errorf("soft delete credential: %w", err)
	}

	s.auditLog(ctx, &userID, AuditCredentialDeleted, "credential", id.String(), ip, ua, true, nil)

	s.logger.Info("credential soft-deleted",
		slog.String("credential_id", id.String()))

	return nil
}

// Test validates bank credentials by attempting a login via the scraper.
func (s *CredentialService) Test(ctx context.Context, cred PlaintextCredential) error {
	if s.tester == nil {
		return fmt.Errorf("credential testing not configured")
	}
	return s.tester.TestCredentials(ctx, cred.BankCode, cred.Fields)
}

// encryptFields marshals credential fields to JSON and encrypts with envelope encryption.
func (s *CredentialService) encryptFields(fields map[string]string) (encData, encDEK []byte, err error) {
	plaintext, err := json.Marshal(fields)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal fields: %w", err)
	}
	encData, encDEK, err = crypto.Seal(s.mk, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt fields: %w", err)
	}
	return encData, encDEK, nil
}

// auditLog writes an audit log entry. Failures are logged but don't block the caller.
func (s *CredentialService) auditLog(ctx context.Context, userID *uuid.UUID, action, targetType, targetID, ip, ua string, success bool, details map[string]any) {
	l := &store.AuditLog{
		UserID:     userID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		IPAddress:  ip,
		UserAgent:  ua,
		Details:    details,
		Success:    success,
	}
	if err := s.audit.Create(ctx, l); err != nil {
		s.logger.Warn("failed to write audit log",
			slog.String("action", action),
			slog.Any("error", err))
	}
}
