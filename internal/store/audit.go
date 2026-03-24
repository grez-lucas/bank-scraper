package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLog represents an entry in the audit_logs table.
type AuditLog struct {
	ID         int64
	Timestamp  time.Time
	UserID     *uuid.UUID
	Action     string
	TargetType string
	TargetID   string
	IPAddress  string
	UserAgent  string
	Details    map[string]any
	Success    bool
}

// AuditFilter specifies criteria for listing audit logs.
type AuditFilter struct {
	UserID   *uuid.UUID
	Action   string
	FromDate *time.Time
	ToDate   *time.Time
	Limit    int
	Offset   int
}

// AuditLogRepository defines operations on the audit_logs table.
type AuditLogRepository interface {
	Create(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, filter AuditFilter) ([]AuditLog, int64, error)
}

// AuditLogRepo implements AuditLogRepository using pgx.
type AuditLogRepo struct {
	pool *pgxpool.Pool
}

// NewAuditLogRepo creates a new AuditLogRepo.
func NewAuditLogRepo(pool *pgxpool.Pool) *AuditLogRepo {
	return &AuditLogRepo{pool: pool}
}

// Create inserts a new audit log entry and populates its generated fields.
func (r *AuditLogRepo) Create(ctx context.Context, l *AuditLog) error {
	query := `
		INSERT INTO audit_logs (user_id, action, target_type, target_id, ip_address, user_agent, details, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, timestamp`

	err := r.pool.QueryRow(ctx, query,
		l.UserID, l.Action, l.TargetType, l.TargetID,
		l.IPAddress, l.UserAgent, l.Details, l.Success,
	).Scan(&l.ID, &l.Timestamp)
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

// List returns audit log entries matching the given filter, along with the total count.
func (r *AuditLogRepo) List(ctx context.Context, filter AuditFilter) ([]AuditLog, int64, error) {
	// Build dynamic WHERE clause
	var conditions []string
	var args []any
	argN := 1

	if filter.UserID != nil {
		conditions = append(conditions, "user_id = $"+strconv.Itoa(argN))
		args = append(args, *filter.UserID)
		argN++
	}
	if filter.Action != "" {
		conditions = append(conditions, "action = $"+strconv.Itoa(argN))
		args = append(args, filter.Action)
		argN++
	}
	if filter.FromDate != nil {
		conditions = append(conditions, "timestamp >= $"+strconv.Itoa(argN))
		args = append(args, *filter.FromDate)
		argN++
	}
	if filter.ToDate != nil {
		conditions = append(conditions, "timestamp <= $"+strconv.Itoa(argN))
		args = append(args, *filter.ToDate)
		argN++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows
	countQuery := "SELECT COUNT(*) FROM audit_logs " + where
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	// Fetch rows with pagination
	limit := 50
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	dataQuery := fmt.Sprintf(
		`SELECT id, timestamp, user_id, action, target_type, target_id,
		        host(ip_address), user_agent, details, success
		 FROM audit_logs %s
		 ORDER BY timestamp DESC
		 LIMIT $%d OFFSET $%d`,
		where, argN, argN+1,
	)
	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(
			&l.ID, &l.Timestamp, &l.UserID, &l.Action, &l.TargetType, &l.TargetID,
			&l.IPAddress, &l.UserAgent, &l.Details, &l.Success,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit logs: %w", err)
	}

	return logs, total, nil
}
