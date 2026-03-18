package debug

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_LazyDirectoryCreation(t *testing.T) {
	baseDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c := New(baseDir, "session-123", logger)

	// Directory should NOT exist yet (lazy creation)
	assert.Equal(t, filepath.Join(baseDir, "session-123"), c.Dir())
	assert.NoDirExists(t, filepath.Join(baseDir, "session-123"))

	// First write creates the directory
	c.HTMLString("<html></html>", "Test", "trigger")
	require.DirExists(t, filepath.Join(baseDir, "session-123"))
}

func TestCollector_NilSafety(t *testing.T) {
	var c *Collector

	// All methods should be no-ops on nil receiver
	c.Screenshot(nil, "Login", "timeout")
	c.HTML(nil, "Login", "timeout")
	c.HTMLString("<html></html>", "Login", "timeout")
	c.JSON(`{"ok":true}`, "Login", "timeout")

	url, dir := c.Snapshot(nil, "Login", "timeout")
	assert.Empty(t, dir)
	assert.Empty(t, url)
	assert.Empty(t, c.Dir())
}

func TestCollector_HTMLString_WritesFile(t *testing.T) {
	baseDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c := New(baseDir, "session-abc", logger)

	c.HTMLString("<html><body>test</body></html>", "GetBalance", "parse-error")

	expected := filepath.Join(baseDir, "session-abc", "GetBalance-parse-error.html")
	require.FileExists(t, expected)
	data, err := os.ReadFile(expected)
	require.NoError(t, err)
	assert.Equal(t, "<html><body>test</body></html>", string(data))
}

func TestCollector_JSON_WritesFile(t *testing.T) {
	baseDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c := New(baseDir, "session-xyz", logger)

	c.JSON(`{"key":"value"}`, "GetBalance", "accounts-timeout-diag")

	expected := filepath.Join(baseDir, "session-xyz", "GetBalance-accounts-timeout-diag.json")
	require.FileExists(t, expected)
	data, err := os.ReadFile(expected)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, string(data))
}

func TestCollector_HTMLString_EmptyNoOp(t *testing.T) {
	baseDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c := New(baseDir, "session-empty", logger)

	c.HTMLString("", "GetBalance", "parse-error")

	// Directory should not have been created (empty string is a no-op)
	assert.NoDirExists(t, filepath.Join(baseDir, "session-empty"))
}
