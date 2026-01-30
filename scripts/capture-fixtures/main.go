package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Pages to capture for each bank
var capturePages = []PageCapture{
	{Name: "login_page", Instructions: "Navigate to the login page (don't login yet)"},
	{Name: "login_error", Instructions: "Enter INVALID credentials and submit"},
	{Name: "dashboard", Instructions: "Login with VALID credentials, wait for dashboard"},
	{Name: "balance_pen", Instructions: "Navigate to a PEN account balance page"},
	{Name: "balance_usd", Instructions: "Navigate to a USD account balance page"},
	{Name: "transactions", Instructions: "Navigate to transactions list (last 7 days)"},
	{Name: "transactions_empty", Instructions: "Navigate to an account with no recent transactions (or skip)"},
}

type PageCapture struct {
	Name         string
	Instructions string
}

func main() {
	bankCode := flag.String("bank", "", "Bank code: bbva, interbank, bcp")
	outputDir := flag.String("output", "", "Output directory (default: internal/scraper/bank/{bank}/testdata/fixtures)")
	flag.Parse()

	if *bankCode == "" {
		fmt.Println("Usage: go run main.go -bank=bbva")
		os.Exit(1)
	}

	// Set output directory
	outDir := *outputDir
	if outDir == "" {
		outDir = filepath.Join("internal", "scraper", "bank", *bankCode, "testdata", "fixtures")
	}

	// Create output directory
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║           BANK FIXTURE CAPTURE TOOL                            ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Bank: %-54s  ║\n", strings.ToUpper(*bankCode))
	fmt.Printf("║  Output: %-52s  ║\n", outDir)
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Launch visible browser
	url := launcher.New().
		Headless(false).
		Devtools(false).
		MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()

	defer browser.MustClose()

	// Create initial page
	page := browser.MustPage("")

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("📋 Instructions:")
	fmt.Println("   - A browser window has opened")
	fmt.Println("   - Follow the prompts below")
	fmt.Println("   - Press ENTER after completing each step")
	fmt.Println("   - Type 'skip' to skip a page")
	fmt.Println("   - Type 'quit' to exit")
	fmt.Println()

	for _, capture := range capturePages {
		fmt.Println("────────────────────────────────────────────────────────────────")
		fmt.Printf("📄 Capturing: %s.html\n", capture.Name)
		fmt.Printf("📝 Instructions: %s\n", capture.Instructions)
		fmt.Print("   Press ENTER when ready (or 'skip'/'quit'): ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "quit" {
			fmt.Println("\n👋 Exiting...")
			break
		}

		if input == "skip" {
			fmt.Printf("   ⏭️  Skipped %s\n\n", capture.Name)
			continue
		}

		// Wait a moment for any dynamic content
		domDiffPercentage := 0.05
		domDiffDuration := 3 * time.Second

		page.WaitDOMStable(domDiffDuration, domDiffPercentage)

		// Capture HTML
		html, err := page.HTML()
		if err != nil {
			fmt.Printf("   ❌ Error getting HTML: %v\n\n", err)
			continue
		}

		// Save HTML
		htmlPath := filepath.Join(outDir, capture.Name+".html")
		if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
			fmt.Printf("   ❌ Error saving HTML: %v\n\n", err)
			continue
		}

		// Capture screenshot
		screenshotPath := filepath.Join(outDir, capture.Name+".png")
		page.MustScreenshot(screenshotPath)

		// Get page URL for reference
		pageURL := page.MustInfo().URL

		fmt.Printf("   ✅ Saved: %s\n", htmlPath)
		fmt.Printf("   📸 Screenshot: %s\n", screenshotPath)
		fmt.Printf("   🔗 URL: %s\n\n", pageURL)
	}

	// Save metadata
	saveMetadata(outDir, *bankCode)

	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Println("✅ Capture complete!")
	fmt.Println()
	fmt.Println("⚠️  IMPORTANT: Sanitize sensitive data before committing!")
	fmt.Println("   Run: go run ./scripts/sanitize-fixtures/main.go -bank=" + *bankCode)
	fmt.Println("════════════════════════════════════════════════════════════════")
}

func saveMetadata(outDir, bankCode string) {
	metadata := fmt.Sprintf(`# Fixture Metadata
bank: %s
captured_at: %s
captured_by: %s

## Files
See .html files in this directory.
Screenshots (.png) provided for visual reference.

## Notes
- These fixtures should be sanitized before committing
- Update when bank portal changes
- Re-run capture if tests start failing
`, bankCode, time.Now().Format(time.RFC3339), os.Getenv("USER"))

	metaPath := filepath.Join(outDir, "README.md")
	os.WriteFile(metaPath, []byte(metadata), 0o644)
}
