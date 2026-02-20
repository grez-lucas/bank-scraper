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
	"github.com/go-rod/stealth"
	browserutil "github.com/grez-lucas/bank-scraper/internal/scraper/browser"
)

// Pages to capture for each bank, ordered by natural portal flow.
var capturePages = []PageCapture{
	// Pre-login
	{Name: "login_page", Instructions: "Navigate to the login page (don't login yet)"},
	{Name: "login_error", Instructions: "Enter INVALID credentials and submit (legacy #aceptar button)"},
	{Name: "login_error_404", Instructions: "Trigger a 404 error page (e.g., navigate to a bad URL while logged out)"},
	{Name: "login_error_403_forbidden", Instructions: "Trigger a 403 Forbidden page (bot detection / Akamai block)"},

	// Post-login
	{Name: "dashboard_news_popup", Instructions: "Login with VALID credentials, wait for dashboard to load. If a news/feature popup appeared, capture it now (or skip)"},
	{Name: "dashboard", Instructions: "Wait for dashboard to load"},
	{Name: "accounts_news_popup", Instructions: "Navigate to the accounts page. If a news/feature popup appeared, capture it now (or skip)"},
	{Name: "accounts_tile", Instructions: "Navigate to the accounts page (default tile/card view). Wait for account cards to load."},
	{Name: "accounts_list", Instructions: "On the accounts page, RELOAD first (previous capture mutated DOM), then click the list-view button (bbva-button-group-item) to switch to list layout. Wait for the table to load."},
	{Name: "transactions", Instructions: "Click 'Ver todos los movimientos' on any account"},
	{Name: "transactions_empty", Instructions: "Navigate to an account with no recent transactions (or skip)"},
	{Name: "transactions_invalid", Instructions: "Navigate to a page with malformed transaction rows (or skip)"},
	{Name: "transactions_load_more", Instructions: "On a transactions page, click 'Ver mas' to load the next page (or skip)"},
	{Name: "logout_modal", Instructions: "Click logout in the sidebar (capture the confirmation modal, don't confirm yet)"},
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
		Bin("/usr/bin/google-chrome").
		Headless(false).
		// CRITICAL: Disable the "Automation" internal flags
		Set("disable-blink-features", "AutomationControlled").
		Set("exclude-switches", "enable-automation").

		// Standard "Human" Args
		Set("no-first-run").
		Set("no-default-browser-check").
		Set("window-size", "1920,1080").
		// Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/60.0.3112.50 Safari/537.36").
		Devtools(false).
		MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()

	defer browser.MustClose()

	// Create initial page
	page := stealth.MustPage(browser)

	// page := browser.MustPage("")

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

		// -- Step 1: Wait for DOM to stabilize, including iframes
		browserutil.WaitForIFrames(page)
		time.Sleep(1 * time.Second)

		// -- Step 2: Screenshot BEFORE DOM modification --
		// Taking the screenshot before the inlining preserves visual fidelity
		screenshotPath := filepath.Join(outDir, capture.Name+".png")
		if buf, err := page.Screenshot(false, nil); err == nil {
			if writeErr := os.WriteFile(screenshotPath, buf, 0o644); writeErr != nil {
				fmt.Printf("   ⚠️  Error saving screenshot: %v\n", err)
			} else {
				fmt.Printf("   📸 Screenshot: %s\n", screenshotPath)
			}
		} else {
			fmt.Printf("   ⚠️  Screenshot failed: %v\n", err)
		}

		// -- Step 3: Flatten shadow DOM + iframes into single HTML
		html, shadowCount, iframeCount, err := browserutil.FlattenShadowDOM(page)
		if err != nil {
			fmt.Printf("   ❌ Error capturing HTML: %v\n\n", err)
			continue
		}

		if shadowCount > 0 || iframeCount > 0 {
			fmt.Printf("   🔲 Flattened %d shadow root(s) and %d iframe(s) into captured HTML\n", shadowCount, iframeCount)
		}

		// -- Step 4: Save HTML fixture --
		htmlPath := filepath.Join(outDir, capture.Name+".html")
		if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
			fmt.Printf("   ❌ Error saving HTML: %v\n\n", err)
			continue
		}

		// Get page URL for reference
		pageURL := page.MustInfo().URL

		fmt.Printf("   ✅ Saved: %s\n", htmlPath)
		fmt.Printf("   🔗 URL: %s\n\n", pageURL)
	}

	// Custom capture mode: let the user capture ad-hoc fixtures by name
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────────────")
	fmt.Println("Custom capture mode:")
	fmt.Println("  Type a fixture name to capture (e.g., \"cookie_popup\")")
	fmt.Println("  Press ENTER with no name to finish")
	fmt.Println("────────────────────────────────────────────────────────────────")

	for {
		fmt.Print("\n📄 Fixture name (or ENTER to finish): ")
		nameInput, _ := reader.ReadString('\n')
		nameInput = strings.TrimSpace(nameInput)

		if nameInput == "" {
			break
		}

		fmt.Printf("   Navigate the browser to the desired state, then press ENTER to capture...")
		reader.ReadString('\n')

		browserutil.WaitForIFrames(page)
		time.Sleep(1 * time.Second)

		screenshotPath := filepath.Join(outDir, nameInput+".png")
		if buf, err := page.Screenshot(false, nil); err == nil {
			if writeErr := os.WriteFile(screenshotPath, buf, 0o644); writeErr != nil {
				fmt.Printf("   ⚠️  Error saving screenshot: %v\n", writeErr)
			} else {
				fmt.Printf("   📸 Screenshot: %s\n", screenshotPath)
			}
		} else {
			fmt.Printf("   ⚠️  Screenshot failed: %v\n", err)
		}

		html, shadowCount, iframeCount, err := browserutil.FlattenShadowDOM(page)
		if err != nil {
			fmt.Printf("   ❌ Error capturing HTML: %v\n", err)
			continue
		}

		if shadowCount > 0 || iframeCount > 0 {
			fmt.Printf("   🔲 Flattened %d shadow root(s) and %d iframe(s) into captured HTML\n", shadowCount, iframeCount)
		}

		htmlPath := filepath.Join(outDir, nameInput+".html")
		if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
			fmt.Printf("   ❌ Error saving HTML: %v\n", err)
			continue
		}

		pageURL := page.MustInfo().URL
		fmt.Printf("   ✅ Saved: %s\n", htmlPath)
		fmt.Printf("   🔗 URL: %s\n", pageURL)
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

## Shadow DOM + Iframe Flattening

Fixtures are captured using FlattenShadowDOM which recursively inlines both
shadow DOM content and iframe documents into a single parseable HTML document.

Shadow root content appears as:

    <custom-element>
      <!-- light DOM children -->
      <div data-shadow-root="true" data-shadow-host="custom-element">
        <style data-from-shadow="true">/* shadow styles */</style>
        <!-- shadow DOM content -->
      </div>
    </custom-element>

Iframe content appears as:

    <div data-captured-iframe="true" data-iframe-src="..." data-iframe-id="...">
      <style data-from-iframe="true">/* iframe styles */</style>
      <!-- iframe body content -->
    </div>

Parse flattened content with goquery:

    doc.Find("[data-shadow-root] .your-selector")
    doc.Find("[data-captured-iframe] .your-selector")

Target a specific shadow host:

    doc.Find("[data-shadow-host='bbva-btge-accounts-solution-page'] .balance")

## Notes
- These fixtures should be sanitized before committing
- Update when bank portal changes
- Re-run capture if tests start failing
`, bankCode, time.Now().Format(time.RFC3339), os.Getenv("USER"))

	metaPath := filepath.Join(outDir, "README.md")
	os.WriteFile(metaPath, []byte(metadata), 0o644)
}
