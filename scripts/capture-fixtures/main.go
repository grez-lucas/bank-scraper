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

// Pages to capture for each bank
var capturePages = []PageCapture{
	{Name: "login_page", Instructions: "Navigate to the login page (don't login yet)"},
	{Name: "login_error", Instructions: "Enter INVALID credentials and submit"},
	{Name: "dashboard", Instructions: "Login with VALID credentials, wait for dashboard"},
	{Name: "balance_pen", Instructions: "Navigate to a PEN account balance page"},
	{Name: "balance_usd", Instructions: "Navigate to a USD account balance page"},
	{Name: "transactions", Instructions: "Navigate to transactions list (last 7 days)"},
	{Name: "transactions_empty", Instructions: "Navigate to an account with no recent transactions (or skip)"},
	{Name: "Logout", Instructions: "Logout of the page before quitting (or skip)"},
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

		// -- Step 3: Inline iframes and capture merged HTML
		html, iframeCount, err := inlineIframesAndCapture(page)
		if err != nil {
			fmt.Printf("   ❌ Error capuring HTML: %v\n\n", err)
			continue
		}

		if iframeCount > 0 {
			fmt.Printf("   🔲 Inlined %d iframe(s) into captured HTML\n", iframeCount)
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

	// Save metadata
	saveMetadata(outDir, *bankCode)

	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Println("✅ Capture complete!")
	fmt.Println()
	fmt.Println("⚠️  IMPORTANT: Sanitize sensitive data before committing!")
	fmt.Println("   Run: go run ./scripts/sanitize-fixtures/main.go -bank=" + *bankCode)
	fmt.Println("════════════════════════════════════════════════════════════════")
}

// inlineIframesAndCapture replaces all <iframe> elements in the live DOM with
// <div data-captured-iframe="true"> containers holding the iframe's body
// content, then returns the full page HTML as a single parsable document.
//
// This solves the core problem: a plain page.HTML() call only returns the
// outer document with empty <iframe> tags — the actual data rendered inside
// iframes (balances, transactions) is lost entirely.
//
// After inlining, the fixture can be parsed with goquery as one flat document:
//
//	doc.Find("[data-captured-iframe] .balance-amount").Text()
//
// DOM modification note: This replaces <iframe> elements in the live DOM.
// This is acceptable because the user navigates to a new page between each
// capture step, discarding the modified DOM.
//
// Returns: merged HTML string, number of iframes found, error.
func inlineIframesAndCapture(page *rod.Page) (string, int, error) {
	// Count iframes before inlining (for reporting)
	iframeCount := 0
	iframes, err := page.Elements("iframe")
	if err != nil {
		return "", 0, fmt.Errorf("failed to get iframe elemenets: %w", err)
	}
	iframeCount += len(iframes)

	if iframeCount == 0 {
		html, htmlErr := page.HTML()
		if htmlErr != nil {
			return "", 0, htmlErr
		}
		return html, 0, nil
	}

	// This will modify the DOM but we should be OK since it will be re-loaded on a page navigation anyway
	_, err = page.Eval(inlineIframesJS)
	if err != nil {
		// Fallback: return outer HTML without inlining.
		// This can happen with cross-origin iframes or restricted pages.
		fmt.Printf("   ⚠️  Could not inline iframes (cross-origin?): %v\n", err)
		html, htmlErr := page.HTML()
		if htmlErr != nil {
			return "", 0, htmlErr
		}
		return html, 0, nil
	}

	// page.HTML() now returns DOM with iframes replaced by their content
	html, err := page.HTML()
	if err != nil {
		return "", 0, err
	}

	return html, iframeCount, nil
}

const inlineIframesJS string = `() => {
	function inlineIframes(root) {
		const iframes = root.querySelectorAll('iframe');

		iframes.forEach((iframe) => {
			try {
				const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
				if (!iframeDoc || !iframeDoc.body) return;

				// Depth-first: inline nested iframes before reading this one's content
				inlineIframes(iframeDoc);

				// Create replacement container in the SAME document as the iframe
				const container = root.createElement('div');
				container.setAttribute('data-captured-iframe', 'true');
				container.setAttribute('data-iframe-src', iframe.src || '');
				container.setAttribute('data-iframe-id', iframe.id || '');
				container.setAttribute('data-iframe-name', iframe.name || '');

				// Preserve layout-relevant dimensions as data attributes
				const w = iframe.getAttribute('width') || iframe.style.width || '';
				const h = iframe.getAttribute('height') || iframe.style.height || '';
				if (w) container.setAttribute('data-iframe-width', w);
				if (h) container.setAttribute('data-iframe-height', h);

				// Build merged content: styles first, then body
				let contentHTML = '';

				// Copy <style> blocks from the iframe's <head>
				if (iframeDoc.head) {
					iframeDoc.head.querySelectorAll('style').forEach((style) => {
						contentHTML += '<style data-from-iframe="true">'
							+ style.textContent + '<\/style>';
					});
				}

				// Append the iframe's body content (the actual data we need)
				contentHTML += iframeDoc.body.innerHTML;

				container.innerHTML = contentHTML;
				iframe.parentNode.replaceChild(container, iframe);
			} catch(e) {
				// Cross-origin or other access restriction — mark but don't crash
				const errDiv = root.createElement('div');
				errDiv.setAttribute('data-captured-iframe', 'true');
				errDiv.setAttribute('data-iframe-error', e.message);
				errDiv.setAttribute('data-iframe-src', iframe.src || '');
				errDiv.textContent = '[iframe not accessible: ' + e.message + ']';
				iframe.parentNode.replaceChild(errDiv, iframe);
			}
		});
	}
	inlineIframes(document);
}`

func saveMetadata(outDir, bankCode string) {
	metadata := fmt.Sprintf(`# Fixture Metadata
bank: %s
captured_at: %s
captured_by: %s

## Files
See .html files in this directory.
Screenshots (.png) provided for visual reference.

## Iframe Handling

Iframe content is automatically inlined during capture as:

    <div data-captured-iframe="true" data-iframe-src="..." data-iframe-name="...">
      <style data-from-iframe="true">/* iframe styles */</style>
      <!-- iframe body content -->
    </div>

Parse inlined iframe content with goquery:

    doc.Find("[data-captured-iframe] .your-selector")

Target a specific iframe by attribute:

    doc.Find("[data-iframe-name='content'] .balance-amount")

## Notes
- These fixtures should be sanitized before committing
- Update when bank portal changes
- Re-run capture if tests start failing
`, bankCode, time.Now().Format(time.RFC3339), os.Getenv("USER"))

	metaPath := filepath.Join(outDir, "README.md")
	os.WriteFile(metaPath, []byte(metadata), 0o644)
}
