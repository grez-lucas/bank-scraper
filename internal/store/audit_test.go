package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogRepo_Create(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	auditRepo := NewAuditLogRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	l := &AuditLog{
		UserID:     &u.ID,
		Action:     "login",
		TargetType: "user",
		TargetID:   u.ID.String(),
		IPAddress:  "10.0.0.1",
		UserAgent:  "TestAgent/1.0",
		Details:    map[string]any{"method": "password"},
		Success:    true,
	}

	err := auditRepo.Create(ctx, l)
	require.NoError(t, err)
	assert.NotZero(t, l.ID)
	assert.False(t, l.Timestamp.IsZero())
}

func TestAuditLogRepo_Create_NilUserID(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	l := &AuditLog{
		UserID:    nil,
		Action:    "system_start",
		IPAddress: "127.0.0.1",
		UserAgent: "system",
		Success:   true,
	}

	err := auditRepo.Create(ctx, l)
	require.NoError(t, err)
	assert.NotZero(t, l.ID)
}

func TestAuditLogRepo_List_NoFilter(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()

	// Create a few logs
	for i := 0; i < 3; i++ {
		require.NoError(t, auditRepo.Create(ctx, &AuditLog{
			Action:    "test_action",
			IPAddress: "10.0.0.1",
			UserAgent: "TestAgent",
			Success:   true,
		}))
	}

	logs, total, err := auditRepo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, logs, 3)
	// Should be ordered by timestamp DESC (most recent first)
	assert.True(t, !logs[0].Timestamp.Before(logs[1].Timestamp))
}

func TestAuditLogRepo_List_FilterByAction(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		Action: "logout", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: false,
	}))

	logs, total, err := auditRepo.List(ctx, AuditFilter{Action: "login"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 2)
	for _, l := range logs {
		assert.Equal(t, "login", l.Action)
	}
}

func TestAuditLogRepo_List_FilterByUserID(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	auditRepo := NewAuditLogRepo(pool)

	u := createTestUser(t, userRepo)
	ctx := context.Background()

	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		UserID: &u.ID, Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		UserID: nil, Action: "system", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))

	logs, total, err := auditRepo.List(ctx, AuditFilter{UserID: &u.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, u.ID, *logs[0].UserID)
}

func TestAuditLogRepo_List_FilterByDateRange(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))

	// Filter for future — should return 0
	future := time.Now().Add(1 * time.Hour)
	logs, total, err := auditRepo.List(ctx, AuditFilter{FromDate: &future})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, logs)

	// Filter for past — should return 1
	past := time.Now().Add(-1 * time.Hour)
	logs, total, err = auditRepo.List(ctx, AuditFilter{FromDate: &past})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
}

func TestAuditLogRepo_List_Pagination(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, auditRepo.Create(ctx, &AuditLog{
			Action: "test", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
		}))
	}

	// Page 1: limit 2, offset 0
	logs, total, err := auditRepo.List(ctx, AuditFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, logs, 2)

	// Page 2: limit 2, offset 2
	logs, total, err = auditRepo.List(ctx, AuditFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, logs, 2)
}

func TestAuditLogRepo_Immutability(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	l := &AuditLog{
		Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}
	require.NoError(t, auditRepo.Create(ctx, l))

	// UPDATE should silently do nothing (PostgreSQL RULE)
	_, err := pool.Exec(ctx, "UPDATE audit_logs SET action = 'hacked' WHERE id = $1", l.ID)
	require.NoError(t, err) // No error, but RULE prevents the update

	// Verify the row is unchanged
	logs, _, err := auditRepo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, "login", logs[0].Action)

	// DELETE should silently do nothing (PostgreSQL RULE)
	_, err = pool.Exec(ctx, "DELETE FROM audit_logs WHERE id = $1", l.ID)
	require.NoError(t, err)

	// Verify the row still exists
	logs, total, err := auditRepo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
}

func TestAuditLogRepo_DetailsJSONB(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	details := map[string]any{
		"bank_code": "BBVA",
		"attempts":  float64(3), // JSON numbers are float64
	}
	l := &AuditLog{
		Action:    "credential_test",
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent",
		Details:   details,
		Success:   false,
	}
	require.NoError(t, auditRepo.Create(ctx, l))

	logs, _, err := auditRepo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "BBVA", logs[0].Details["bank_code"])
	assert.Equal(t, float64(3), logs[0].Details["attempts"])
}

func TestAuditLogRepo_IPAddress(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	auditRepo := NewAuditLogRepo(pool)

	ctx := context.Background()
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		Action: "login", IPAddress: "192.168.1.100", UserAgent: "TestAgent", Success: true,
	}))

	logs, _, err := auditRepo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.100", logs[0].IPAddress)
}

func TestAuditLogRepo_List_CombinedFilters(t *testing.T) {
	pool := testPool(t)
	truncateTables(t, pool)
	userRepo := NewUserRepo(pool)
	auditRepo := NewAuditLogRepo(pool)

	u := createTestUser(t, userRepo)
	otherID := uuid.New() // non-existent user ID — we won't insert with it

	ctx := context.Background()
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		UserID: &u.ID, Action: "login", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))
	require.NoError(t, auditRepo.Create(ctx, &AuditLog{
		UserID: &u.ID, Action: "logout", IPAddress: "10.0.0.1", UserAgent: "TestAgent", Success: true,
	}))

	// Filter: user + action
	logs, total, err := auditRepo.List(ctx, AuditFilter{UserID: &u.ID, Action: "login"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)

	// Filter: wrong user — should return 0
	logs, total, err = auditRepo.List(ctx, AuditFilter{UserID: &otherID})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, logs)
}
