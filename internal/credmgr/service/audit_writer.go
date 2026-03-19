package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// AuditWriter writes audit log entries to the database.
// Failures are logged via slog but never block the caller.
type AuditWriter struct {
	audit  store.AuditLogRepository
	logger *slog.Logger
}

// NewAuditWriter creates a new AuditWriter.
func NewAuditWriter(audit store.AuditLogRepository, logger *slog.Logger) *AuditWriter {
	return &AuditWriter{audit: audit, logger: logger}
}

// Log writes an audit log entry. Best-effort — errors are logged, not returned.
func (w *AuditWriter) Log(ctx context.Context, userID *uuid.UUID, action, targetType, targetID, ip, ua string, success bool, details map[string]any) {
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
	if err := w.audit.Create(ctx, l); err != nil {
		w.logger.Warn("failed to write audit log",
			slog.String("action", action),
			slog.Any("error", err))
	}
}
