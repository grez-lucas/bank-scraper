package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var sanitizePatterns = []struct {
	Pattern     *regexp.Regexp
	Replacement string
	Description string
}{
	// Account numbers (various formats)
	// BBVA Accounts
	{
		regexp.MustCompile(`\b\d{4}-\d{4}-\d{2}-\d{8}\b`),
		`XXXX-XXXX-XX-XXXXXXXX`,
		"Account number (bbva format)",
	},

	// Nombres en espanhol
	{
		regexp.MustCompile(`(?i)(Hola)\s+[A-ZÁÉÍÓÚÑ][a-záéíóúñ]+\s+[A-ZÁÉÍÓÚÑ][a-záéíóúñ]+`),
		"$1 NOMBRE APELLIDO",
		"Full name with title",
	},

	// Session tokens / CSRF tokens
	{
		regexp.MustCompile(`(?i)(token|csrf|session)["\s:=]+["']?[a-zA-Z0-9_-]{20,}["']?`),
		`$1="REDACTED"`,
		"Token",
	},

	// Cookies in HTML
	{
		regexp.MustCompile(`(?i)document\.cookie\s*=\s*["'][^"']+["']`),
		`document.cookie="REDACTED"`,
		"Cookie",
	},
}

func main() {
	bankCode := flag.String("bank", "", "Bank code: bbva, interbank, bcp")
	dryRun := flag.Bool("dry-run", false, "Show what would be changed without modifying files")
	flag.Parse()

	if *bankCode == "" {
		fmt.Println("Usage: go run main.go -bank=bbva [--dry-run]")
		os.Exit(1)
	}

	fixturesDir := filepath.Join("internal", "scraper", "bank", *bankCode, "testdata", "fixtures")

	files, err := filepath.Glob(filepath.Join(fixturesDir, "*.html"))
	if err != nil || len(files) == 0 {
		fmt.Printf("No HTML files found in %s\n", fixturesDir)
		os.Exit(1)
	}

	fmt.Printf("🔒 Sanitizing fixtures for %s\n", *bankCode)
	if *dryRun {
		fmt.Println("    (DRY RUN - no files will be modified)")
	}
	fmt.Println()

	for _, file := range files {
		sanitizeFile(file, *dryRun)
	}

	fmt.Println()
	fmt.Println("✅ Sanitization complete!")
	if *dryRun {
		fmt.Println("    Run without --dry-run to apply changes")
	}
}

func sanitizeFile(path string, dryRun bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("❌ Error reading %s: %v\n", path, err)
		return
	}

	original := string(content)
	sanitized := original
	changes := []string{}

	for _, pattern := range sanitizePatterns {
		if pattern.Pattern.MatchString(sanitized) {
			matches := pattern.Pattern.FindAllString(sanitized, -1)
			sanitized = pattern.Pattern.ReplaceAllString(sanitized, pattern.Replacement)
			changes = append(changes, fmt.Sprintf("  - %s: %d matched", pattern.Description, len(matches)))
		}
	}

	filename := filepath.Base(path)

	if len(changes) == 0 {
		fmt.Printf("📄 %s: No sensitive data found\n", filename)
		return
	}

	fmt.Printf("📄 %s: Found sensitive data\n", filename)
	for _, change := range changes {
		fmt.Println(change)
	}

	// Check if we should write to the original file
	if !dryRun {
		if err := os.WriteFile(path, []byte(sanitized), 0o644); err != nil {
			fmt.Printf("    ❌ Error writing %s: %v\n ", path, err)
		} else {
			fmt.Println("    ✅ Sanitized and saved")
		}
	}
}
