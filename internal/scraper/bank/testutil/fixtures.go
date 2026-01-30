package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// LoadFixture reads an HTML fixture file for the given bank
func LoadFixture(t *testing.T, bankCode, name string) string {
	t.Helper()

	// Get path relative to thjis file
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filename)) // up to bank/

	path := filepath.Join(baseDir, bankCode, "testdata", "fixtures", name+".html")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load fixture %s/%s: %v", bankCode, name, err)
	}

	return string(data)
}

// MustLoadFixture is like LoadFixture but panics on error (for non-test use)
func MustLoadFixture(t *testing.T, bankCode, name string) string {
	t.Helper()

	// Get path relative to thjis file
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filename)) // up to bank/

	path := filepath.Join(baseDir, bankCode, "testdata", "fixtures", name+".html")

	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	return string(data)
}
