package debug

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartOp_LogsStart(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	_ = StartOp(logger, "Login", slog.String("bank", "bbva"))

	entry := parseFirstLogEntry(t, buf.String())
	assert.Equal(t, "starting Login", entry["msg"])
	assert.Equal(t, "Login", entry["operation"])
	assert.Equal(t, "bbva", entry["bank"])
}

func TestOpLogger_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	op := StartOp(logger, "GetBalance")
	op.Success(slog.Int("account_count", 2))

	entries := parseAllLogEntries(t, buf.String())
	require.Len(t, entries, 2)

	entry := entries[1]
	assert.Equal(t, "GetBalance completed", entry["msg"])
	assert.Equal(t, "GetBalance", entry["operation"])
	assert.NotNil(t, entry["duration_ms"])
	assert.Equal(t, float64(2), entry["account_count"])
}

func TestOpLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	op := StartOp(logger, "GetTransactions", slog.String("account_id", "123"))
	op.Error("timeout", errors.New("context deadline exceeded"),
		slog.String("url", "https://example.com"))

	entries := parseAllLogEntries(t, buf.String())
	require.Len(t, entries, 2)

	entry := entries[1]
	assert.Equal(t, "GetTransactions: timeout", entry["msg"])
	assert.Equal(t, "GetTransactions", entry["operation"])
	assert.Equal(t, "ERROR", entry["level"])
	assert.NotNil(t, entry["duration_ms"])
	assert.Equal(t, "context deadline exceeded", entry["error"])
	assert.Equal(t, "https://example.com", entry["url"])
}

func TestOpLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	op := StartOp(logger, "Logout")
	op.Warn("DOM unstable after click")

	entries := parseAllLogEntries(t, buf.String())
	require.Len(t, entries, 2)

	entry := entries[1]
	assert.Equal(t, "Logout: DOM unstable after click", entry["msg"])
	assert.Equal(t, "WARN", entry["level"])
	assert.NotNil(t, entry["duration_ms"])
}

func TestOpLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	op := StartOp(logger, "GetTransactions")
	op.Info("pagination click", slog.Int("iteration", 3))

	entries := parseAllLogEntries(t, buf.String())
	require.Len(t, entries, 2)

	entry := entries[1]
	assert.Equal(t, "GetTransactions: pagination click", entry["msg"])
	assert.Equal(t, "GetTransactions", entry["operation"])
	assert.Equal(t, float64(3), entry["iteration"])
}

func parseFirstLogEntry(t *testing.T, output string) map[string]any {
	t.Helper()
	entries := parseAllLogEntries(t, output)
	require.NotEmpty(t, entries)
	return entries[0]
}

func parseAllLogEntries(t *testing.T, output string) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}
	return entries
}
