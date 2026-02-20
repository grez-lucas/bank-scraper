# Capturing HTML Fixtures

Author: Lucas Grez
Created time: January 29, 2026 4:39 PM
Last edited by: Lucas Grez
Last updated time: February 20, 2026

# Capturing HTML Fixtures for Bank Scraper Testing

**Purpose:** Get real HTML from bank portals to use in unit tests

**Time required:** ~15 minutes per bank (initial setup)

---

## Overview: Three Methods

| Method | Effort | Best For |
| --- | --- | --- |
| **1. Browser DevTools** | Low | Quick one-off captures |
| **2. Semi-automated Rod script** | Medium | Repeatable captures with guidance |
| **3. Fully automated capture** | High | CI/CD fixture refresh |

**Recommendation:** Start with Method 2 (semi-automated). It's the best balance of speed and repeatability.

---

## Shadow DOM Pages (BBVA 2026+)

BBVA's post-login pages use Web Components (Polymer/Cells framework) with deeply nested shadow roots. A plain `document.documentElement.outerHTML` — or `copy(document.documentElement.outerHTML)` in DevTools — only returns **empty custom element shells**. The actual data (accounts, balances, transactions) is hidden behind multiple layers of shadow DOM, often with iframes interleaved.

**This affects all three capture methods.** Before copying/saving HTML, you must run the shadow DOM flattener to inline shadow content into the regular DOM.

### Why plain HTML capture fails

```
What outerHTML gives you:            What the parser needs:

<bbva-btge-app-template>             <bbva-btge-app-template>
  <bbva-btge-accounts-page>            <bbva-btge-accounts-page>
    <!-- empty shell -->                 <div data-shadow-root="true">
  </bbva-btge-accounts-page>               <table id="data-table">
</bbva-btge-app-template>                    <tr><td>S/ 1,234.56</td></tr>
                                           </table>
                                         </div>
                                       </bbva-btge-accounts-page>
                                     </bbva-btge-app-template>
```

Shadow roots are not part of the serialized DOM. They must be walked and inlined explicitly.

### How to flatten from the browser console

Open DevTools (F12), go to the **Console** tab, and paste the following snippet. It walks every shadow root and iframe in the page, inlines their content into the regular DOM with marker attributes, and copies the result to your clipboard.

```javascript
// Shadow DOM + iframe flattener — paste into DevTools console.
// Mirrors the logic in internal/scraper/browser/shadow.go.
(() => {
  const MAX_DEPTH = 100;
  let shadowCount = 0;
  let iframeCount = 0;

  function flattenNode(node, depth) {
    if (depth > MAX_DEPTH) return;
    const children = Array.from(node.childNodes);
    for (const child of children) {
      if (child.nodeType === Node.ELEMENT_NODE) {
        flattenElement(child, depth);
      }
    }
  }

  function flattenElement(el, depth) {
    if (depth > MAX_DEPTH) return;
    if (el.tagName === 'IFRAME') { inlineIframe(el, depth); return; }
    if (el.shadowRoot) { flattenShadow(el, depth); return; }
    flattenNode(el, depth + 1);
  }

  function flattenShadow(host, depth) {
    const shadow = host.shadowRoot;
    if (!shadow) return;

    // Flatten light DOM children first (slotted Polymer components)
    for (const child of Array.from(host.childNodes)) {
      if (child.nodeType === Node.ELEMENT_NODE) {
        flattenElement(child, depth + 1);
      }
    }

    // Then flatten shadow children
    for (const child of Array.from(shadow.childNodes)) {
      if (child.nodeType === Node.ELEMENT_NODE) {
        flattenElement(child, depth + 1);
      }
    }

    // Serialize flattened shadow content into a marker div
    const container = document.createElement('div');
    container.setAttribute('data-shadow-root', 'true');
    container.setAttribute('data-shadow-host', host.tagName.toLowerCase());

    shadow.querySelectorAll('style').forEach(style => {
      const s = document.createElement('style');
      s.setAttribute('data-from-shadow', 'true');
      s.textContent = style.textContent;
      container.appendChild(s);
    });

    for (const child of Array.from(shadow.childNodes)) {
      if (child.nodeType === Node.ELEMENT_NODE && child.tagName === 'STYLE') continue;
      try { container.appendChild(child.cloneNode(true)); } catch(e) {}
    }

    shadowCount++;
    host.appendChild(container);
  }

  function inlineIframe(iframe, depth) {
    try {
      const doc = iframe.contentDocument || (iframe.contentWindow && iframe.contentWindow.document);
      if (!doc || !doc.documentElement) {
        const err = iframe.ownerDocument.createElement('div');
        err.setAttribute('data-captured-iframe', 'true');
        err.setAttribute('data-iframe-error', 'no contentDocument');
        err.setAttribute('data-iframe-src', iframe.src || '');
        err.textContent = '[iframe not accessible]';
        iframe.parentNode.replaceChild(err, iframe);
        return;
      }
      flattenNode(doc.documentElement, depth + 1);
      const container = iframe.ownerDocument.createElement('div');
      container.setAttribute('data-captured-iframe', 'true');
      container.setAttribute('data-iframe-src', iframe.src || '');
      container.setAttribute('data-iframe-id', iframe.id || '');
      container.setAttribute('data-iframe-name', iframe.name || '');
      if (doc.head) {
        doc.head.querySelectorAll('style').forEach(style => {
          const s = iframe.ownerDocument.createElement('style');
          s.setAttribute('data-from-iframe', 'true');
          s.textContent = style.textContent;
          container.appendChild(s);
        });
      }
      if (doc.body) container.innerHTML += doc.body.innerHTML;
      iframeCount++;
      iframe.parentNode.replaceChild(container, iframe);
    } catch(e) {
      try {
        const err = iframe.ownerDocument.createElement('div');
        err.setAttribute('data-captured-iframe', 'true');
        err.setAttribute('data-iframe-error', e.message);
        err.setAttribute('data-iframe-src', iframe.src || '');
        err.textContent = '[iframe error: ' + e.message + ']';
        iframe.parentNode.replaceChild(err, iframe);
      } catch(e2) {}
    }
  }

  flattenNode(document.documentElement, 0);

  const html = document.documentElement.outerHTML;
  copy(html);
  console.log(
    `%c Flattened: ${shadowCount} shadow roots, ${iframeCount} iframes. HTML copied to clipboard (${html.length} chars).`,
    'color: green; font-weight: bold'
  );
})();
```

After running, paste from your clipboard into a `.html` file. The HTML now contains:
- `<div data-shadow-root="true" data-shadow-host="...">` — inlined shadow content
- `<div data-captured-iframe="true" data-iframe-src="...">` — inlined iframe content
- `<style data-from-shadow="true">` — styles extracted from shadow roots

### When do you need this?

| Page | Needs flattening? | Why |
| --- | --- | --- |
| Login page (pre-login) | No | Standard HTML, no Web Components |
| Login error page | No | Same as login page |
| Dashboard (post-login) | **Yes** | Polymer/Cells Web Components |
| Balance pages | **Yes** | Nested shadow roots with data tables |
| Transactions page | **Yes** | Shadow DOM + possibly iframes |
| Logout modal | **Yes** | Web Component dialog |

### Verifying the capture

After flattening, search the saved HTML for your expected selectors:

```bash
# Should find data-shadow-root markers
grep -c 'data-shadow-root' dashboard.html

# Should find actual content (not empty shells)
grep -c 'data-table\|saldo\|monto' balance_pen.html

# Should NOT have empty custom elements with zero children
# (indicates flattening didn't run or missed a shadow root)
```

### Important notes

- **Flattening mutates the live DOM.** The page will look the same visually, but the DOM structure changes. Reload the page if you need to interact with it again after capturing.
- **Cross-origin iframes** (e.g., analytics, tracking) will show `[iframe not accessible]` markers. This is expected — those iframes don't contain bank data.
- **The script must run after the page is fully loaded.** Wait for all spinners/loading indicators to disappear before running.
- **The canonical implementation** lives in `internal/scraper/browser/shadow.go` (`FlattenShadowDOM`). If the JS snippet above drifts out of sync, regenerate it from there.

---

## Method 1: Browser DevTools (Quickest)

### Steps

1. **Open bank website in Chrome/Firefox**
2. **Login manually**
3. **Open DevTools** (F12 or Cmd+Option+I)
4. **Navigate to the page you want** (e.g., balance page)
5. **Wait for full load** (watch Network tab until quiet)
6. **Copy the HTML:**

```
Option A: Elements panel
- Right-click on <html> tag
- Select "Copy" → "Copy outerHTML"
- Paste into balance_pen.html

Option B: Console
- Type: copy(document.documentElement.outerHTML)
- Paste into file

```

> **Shadow DOM warning:** Options A and B only capture the outer DOM. For post-login BBVA pages (dashboard, balances, transactions), you **must** run the shadow DOM flattener snippet from the "Shadow DOM Pages" section above first. Then use `copy(document.documentElement.outerHTML)` — the flattened content will already be inlined.

7. **Save screenshot** (for visual reference)
8. **Sanitize sensitive data** (see section below)

### Pros & Cons

✅ No code required

✅ Works immediately

❌ Manual and tedious for multiple pages

❌ Easy to forget steps

❌ Hard to reproduce exactly

❌ Misses shadow DOM content unless flattener is run first

---

## Method 2: Semi-Automated Rod Script (Recommended)

This script opens a visible browser, guides you through the capture process, and automatically saves HTML at each step.

### scripts/capture-fixtures/main.go

```go
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
	"github.com/go-rod/rod/lib/proto"
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
	if err := os.MkdirAll(outDir, 0755); err != nil {
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
		time.Sleep(500 * time.Millisecond)

		// Capture HTML
		html, err := page.HTML()
		if err != nil {
			fmt.Printf("   ❌ Error getting HTML: %v\n\n", err)
			continue
		}

		// Save HTML
		htmlPath := filepath.Join(outDir, capture.Name+".html")
		if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
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
	os.WriteFile(metaPath, []byte(metadata), 0644)
}

```

### How to Use

```bash
# Run the capture tool
go run ./scripts/capture-fixtures/main.go -bank=bbva

# Output:
# ╔════════════════════════════════════════════════════════════════╗
# ║           BANK FIXTURE CAPTURE TOOL                            ║
# ╠════════════════════════════════════════════════════════════════╣
# ║  Bank: BBVA                                                    ║
# ║  Output: internal/scraper/bank/bbva/testdata/fixtures          ║
# ╚════════════════════════════════════════════════════════════════╝
#
# 📋 Instructions:
#    - A browser window has opened
#    - Follow the prompts below
#    ...
#
# ────────────────────────────────────────────────────────────────
# 📄 Capturing: login_page.html
# 📝 Instructions: Navigate to the login page (don't login yet)
#    Press ENTER when ready (or 'skip'/'quit'):

```

### Pros & Cons

✅ Guided process, hard to forget steps

✅ Automatically saves HTML + screenshots

✅ Repeatable with same steps

✅ Captures fully-rendered JavaScript content

❌ Still requires manual navigation

---

## Method 3: Fully Automated Capture (Advanced)

For CI/CD or frequent updates, automate the entire capture process.

### scripts/auto-capture/bbva.go

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

const (
	bbvaLoginURL = "https://www.bbva.pe/personas/login"
	outputDir    = "internal/scraper/bank/bbva/testdata/fixtures"
)

type BBVACapture struct {
	browser  *rod.Browser
	page     *rod.Page
	username string
	password string
}

func main() {
	username := os.Getenv("BBVA_TEST_USER")
	password := os.Getenv("BBVA_TEST_PASS")
	accountPEN := os.Getenv("BBVA_TEST_ACCOUNT_PEN")
	accountUSD := os.Getenv("BBVA_TEST_ACCOUNT_USD")

	if username == "" || password == "" {
		fmt.Println("Error: Set BBVA_TEST_USER and BBVA_TEST_PASS environment variables")
		os.Exit(1)
	}

	os.MkdirAll(outputDir, 0755)

	capture := NewBBVACapture(username, password)
	defer capture.Close()

	// Capture login page (before login)
	capture.CaptureLoginPage()

	// Capture login error
	capture.CaptureLoginError()

	// Login successfully
	capture.Login()

	// Capture dashboard
	capture.CaptureDashboard()

	// Capture balances
	if accountPEN != "" {
		capture.CaptureBalance(accountPEN, "balance_pen")
	}
	if accountUSD != "" {
		capture.CaptureBalance(accountUSD, "balance_usd")
	}

	// Capture transactions
	if accountPEN != "" {
		capture.CaptureTransactions(accountPEN, "transactions")
	}

	fmt.Println("✅ All fixtures captured!")
}

func NewBBVACapture(username, password string) *BBVACapture {
	url := launcher.New().
		Headless(true).
		MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()

	return &BBVACapture{
		browser:  browser,
		username: username,
		password: password,
	}
}

func (c *BBVACapture) CaptureLoginPage() {
	fmt.Println("📄 Capturing login page...")
	c.page = c.browser.MustPage(bbvaLoginURL)
	c.page.MustWaitLoad()
	c.savePage("login_page")
}

func (c *BBVACapture) CaptureLoginError() {
	fmt.Println("📄 Capturing login error...")
	c.page.MustElement("#username").MustInput("invalid_user")
	c.page.MustElement("#password").MustInput("invalid_pass")
	c.page.MustElement("#btn-login").MustClick()

	// Wait for error message
	c.page.MustElement(".error-message")
	c.savePage("login_error")

	// Refresh for clean state
	c.page.MustNavigate(bbvaLoginURL)
	c.page.MustWaitLoad()
}

func (c *BBVACapture) Login() {
	fmt.Println("🔐 Logging in...")
	c.page.MustElement("#username").MustInput(c.username)
	c.page.MustElement("#password").MustInput(c.password)
	c.page.MustElement("#btn-login").MustClick()
	c.page.MustWaitLoad()

	// Wait for dashboard indicator
	time.Sleep(2 * time.Second) // Allow JS to render
}

func (c *BBVACapture) CaptureDashboard() {
	fmt.Println("📄 Capturing dashboard...")
	c.savePage("dashboard")
}

func (c *BBVACapture) CaptureBalance(accountID, name string) {
	fmt.Printf("📄 Capturing %s...\n", name)

	// Navigate to account balance (adjust URL pattern for actual bank)
	balanceURL := fmt.Sprintf("https://www.bbva.pe/personas/cuentas/%s", accountID)
	c.page.MustNavigate(balanceURL)
	c.page.MustWaitLoad()
	time.Sleep(1 * time.Second) // Allow JS to render

	c.savePage(name)
}

func (c *BBVACapture) CaptureTransactions(accountID, name string) {
	fmt.Printf("📄 Capturing %s...\n", name)

	// Navigate to transactions (adjust URL pattern for actual bank)
	txURL := fmt.Sprintf("https://www.bbva.pe/personas/cuentas/%s/movimientos", accountID)
	c.page.MustNavigate(txURL)
	c.page.MustWaitLoad()
	time.Sleep(1 * time.Second)

	c.savePage(name)
}

func (c *BBVACapture) savePage(name string) {
	html, err := c.page.HTML()
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
		return
	}

	htmlPath := filepath.Join(outputDir, name+".html")
	os.WriteFile(htmlPath, []byte(html), 0644)

	screenshotPath := filepath.Join(outputDir, name+".png")
	c.page.MustScreenshot(screenshotPath)

	fmt.Printf("   ✅ Saved: %s\n", htmlPath)
}

func (c *BBVACapture) Close() {
	c.browser.MustClose()
}

```

### Pros & Cons

✅ Fully repeatable

✅ Can run in CI/CD

✅ Fast once written

❌ Requires knowing exact selectors upfront (chicken-and-egg)

❌ Breaks when bank changes

**Recommendation:** Use Method 2 first to discover selectors, then optionally build Method 3.

---

## Sanitizing Fixtures (Critical!)

Before committing HTML fixtures, **remove sensitive data**:

### scripts/sanitize-fixtures/main.go

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Patterns to sanitize
var sanitizePatterns = []struct {
	Pattern     *regexp.Regexp
	Replacement string
	Description string
}{
	// Account numbers (various formats)
	{regexp.MustCompile(`\b\d{3}-\d{7}-\d{2}\b`), "XXX-XXXXXXX-XX", "Account number (format 1)"},
	{regexp.MustCompile(`\b\d{14,20}\b`), "XXXXXXXXXXXX", "Account number (long)"},

	// Names (common patterns in Spanish bank sites)
	{regexp.MustCompile(`(?i)(Sr\.|Sra\.|Don|Doña)\s+[A-ZÁÉÍÓÚÑ][a-záéíóúñ]+\s+[A-ZÁÉÍÓÚÑ][a-záéíóúñ]+`), "$1 NOMBRE APELLIDO", "Full name with title"},

	// Email addresses
	{regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "usuario@ejemplo.com", "Email"},

	// Phone numbers (Peru format)
	{regexp.MustCompile(`\+?51\s?\d{9}`), "+51 999999999", "Phone (Peru)"},
	{regexp.MustCompile(`\b9\d{8}\b`), "999999999", "Mobile (Peru)"},

	// DNI (Peru national ID)
	{regexp.MustCompile(`\b\d{8}\b`), "99999999", "DNI"},

	// Specific amounts (optional - you might want to keep these)
	// {regexp.MustCompile(`S/\.?\s*[\d,]+\.\d{2}`), "S/ X,XXX.XX", "Amount PEN"},

	// Session tokens / CSRF tokens
	{regexp.MustCompile(`(?i)(token|csrf|session)["\s:=]+["']?[a-zA-Z0-9_-]{20,}["']?`), `$1="REDACTED"`, "Token"},

	// Cookies in HTML
	{regexp.MustCompile(`(?i)document\.cookie\s*=\s*["'][^"']+["']`), `document.cookie="REDACTED"`, "Cookie"},
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

	fmt.Printf("🔒 Sanitizing fixtures for %s\n", strings.ToUpper(*bankCode))
	if *dryRun {
		fmt.Println("   (DRY RUN - no files will be modified)")
	}
	fmt.Println()

	for _, file := range files {
		sanitizeFile(file, *dryRun)
	}

	fmt.Println()
	fmt.Println("✅ Sanitization complete!")
	if *dryRun {
		fmt.Println("   Run without --dry-run to apply changes")
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
			changes = append(changes, fmt.Sprintf("  - %s: %d matches", pattern.Description, len(matches)))
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

	if !dryRun {
		if err := os.WriteFile(path, []byte(sanitized), 0644); err != nil {
			fmt.Printf("   ❌ Error writing: %v\n", err)
		} else {
			fmt.Println("   ✅ Sanitized and saved")
		}
	}
}

```

### Usage

```bash
# Preview what will be sanitized
go run ./scripts/sanitize-fixtures/main.go -bank=bbva --dry-run

# Apply sanitization
go run ./scripts/sanitize-fixtures/main.go -bank=bbva

```

---

## What to Capture: Complete Checklist

| Fixture | Purpose | What to Capture |
| --- | --- | --- |
| `login_page.html` | Test login form detection | Login page before entering credentials |
| `login_error.html` | Test error parsing | Page after failed login attempt |
| `dashboard.html` | Test account listing | Main page after successful login |
| `balance_pen.html` | Test PEN balance parsing | Account page showing PEN balance |
| `balance_usd.html` | Test USD balance parsing | Account page showing USD balance |
| `transactions.html` | Test transaction parsing | Transaction list with 5+ items |
| `transactions_empty.html` | Test empty state | Account with no transactions |
| `session_expired.html` | Test session detection | Page shown when session times out |

---

## Directory Structure After Capture

```
internal/scraper/bank/bbva/testdata/
├── fixtures/
│   ├── README.md              # Capture metadata
│   ├── login_page.html        # HTML fixtures
│   ├── login_page.png         # Visual reference
│   ├── login_error.html
│   ├── login_error.png
│   ├── dashboard.html
│   ├── dashboard.png
│   ├── balance_pen.html
│   ├── balance_pen.png
│   ├── balance_usd.html
│   ├── balance_usd.png
│   ├── transactions.html
│   ├── transactions.png
│   └── transactions_empty.html
└── recordings/                # (Optional) Rod traces for replay
    └── full_session.trace/

```

---

## Loading Fixtures in Tests

### Helper Function

```go
// internal/scraper/bank/testutil/fixtures.go
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

	// Get path relative to this file
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filename)) // Up to bank/

	path := filepath.Join(baseDir, bankCode, "testdata", "fixtures", name+".html")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load fixture %s/%s: %v", bankCode, name, err)
	}

	return string(data)
}

// MustLoadFixture is like LoadFixture but panics on error (for non-test use)
func MustLoadFixture(bankCode, name string) string {
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filename))
	path := filepath.Join(baseDir, bankCode, "testdata", "fixtures", name+".html")

	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}

```

### Using in Parser Tests

```go
// internal/scraper/bank/bbva/parser_test.go
package bbva

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yourcompany/bank-scraper/internal/scraper/bank/testutil"
)

func TestParseBalance_PEN(t *testing.T) {
	// Load fixture
	html := testutil.LoadFixture(t, "bbva", "balance_pen")

	// Test parsing
	balance, err := ParseBalance(html)

	require.NoError(t, err)
	// ... assertions
}

```

---

## Quick Reference: Makefile Commands

```makefile
.PHONY: capture-bbva capture-interbank capture-bcp sanitize-fixtures

# Capture fixtures (semi-automated)
capture-bbva:
	go run ./scripts/capture-fixtures/main.go -bank=bbva

capture-interbank:
	go run ./scripts/capture-fixtures/main.go -bank=interbank

capture-bcp:
	go run ./scripts/capture-fixtures/main.go -bank=bcp

capture-all: capture-bbva capture-interbank capture-bcp

# Sanitize all fixtures
sanitize-fixtures:
	go run ./scripts/sanitize-fixtures/main.go -bank=bbva
	go run ./scripts/sanitize-fixtures/main.go -bank=interbank
	go run ./scripts/sanitize-fixtures/main.go -bank=bcp

# Preview sanitization
sanitize-preview:
	go run ./scripts/sanitize-fixtures/main.go -bank=bbva --dry-run
	go run ./scripts/sanitize-fixtures/main.go -bank=interbank --dry-run
	go run ./scripts/sanitize-fixtures/main.go -bank=bcp --dry-run

```

---

## Summary: Recommended Workflow

```
┌─────────────────────────────────────────────────────────────────────┐
│                 FIXTURE CAPTURE WORKFLOW                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. Run capture tool                                                │
│     └─▶ make capture-bbva                                           │
│                                                                     │
│  2. Follow prompts in browser                                       │
│     └─▶ Login, navigate, press ENTER at each step                   │
│                                                                     │
│  3. Review screenshots                                              │
│     └─▶ Check that correct pages were captured                      │
│                                                                     │
│  4. Sanitize sensitive data                                         │
│     └─▶ make sanitize-fixtures                                      │
│                                                                     │
│  5. Commit fixtures                                                 │
│     └─▶ git add internal/scraper/bank/bbva/testdata/fixtures        │
│                                                                     │
│  6. Write/update parser tests                                       │
│     └─▶ Tests now use real HTML structure                           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

```

**Time investment:**

- Initial capture: ~15 min per bank
- Updates when site changes: ~5 min per bank
- Sanitization: Automated (~10 seconds)

This approach gives you realistic test fixtures without the risk of committing sensitive data or hitting live banks during development.