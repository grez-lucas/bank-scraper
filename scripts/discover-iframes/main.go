// discover-iframes navigates to a bank page and prints the iframe tree,
// probing each frame for known CSS selectors. The output serves as a
// reference for which frame contains which parseable content.
//
// Usage:
//
//	go run ./scripts/discover-iframes -bank=bbva
//
// The script opens a visible browser and prompts you to navigate to each
// page manually. After you press ENTER, it inspects the frame tree and
// prints a report.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	browserutil "github.com/grez-lucas/bank-scraper/internal/scraper/browser"
)

// selectorProbe is a known CSS selector to search for in each frame.
type selectorProbe struct {
	Name     string // Human label (e.g., "Login button")
	Selector string // CSS selector
}

// bbvaProbes are the known selectors from selectors.go to probe for.
// Add new selectors here as you discover them.
var bbvaProbes = []selectorProbe{
	// Login
	{"Company input", "input#empresa"},
	{"User input", "input#usuario"},
	{"Password input", "input#clave_acceso_ux"},
	{"Login button (legacy)", "button#aceptar"},
	{"Login button (senda)", "button#enviarSenda"},
	{"Login error code", "div.error-code.error-title"},
	{"Login error message", "h1.title"},
	{"Login error span", "span#error-message"},

	// Dashboard
	{"Dashboard table", "table#kyop-boby-table"},

	// Balance / Accounts
	{"Accounts table rows", "#tabla-contenedor0_1 tbody tr"},

	// Transactions
	{"Transactions table", "table#tabladatos"},
	{"Transactions rows", "#tabladatos tbody tr"},
	{"No movements error", "div.msj_ico.msj_err"},

	// Cookie / popups
	{"Cookie popup", "[class*='cookie']"},
	{"Cookie accept button", "[class*='cookie'] button"},
}

// pageToInspect defines a page the user should navigate to.
type pageToInspect struct {
	Name         string
	Instructions string
}

var bbvaPages = []pageToInspect{
	{"Login page", "Navigate to the BBVA login page (don't log in yet)"},
	{"Dashboard", "Log in with valid credentials, wait for dashboard to load"},
	{"Accounts page", "Navigate to the accounts page from the sidebar"},
	{"Transactions (full history)", "Click 'Ver todos los movimientos' on any account"},
	{"Logout modal", "Click logout in the sidebar (don't confirm yet)"},
}

func main() {
	bankCode := flag.String("bank", "", "Bank code: bbva")
	flag.Parse()

	if *bankCode == "" {
		fmt.Println("Usage: go run ./scripts/discover-iframes -bank=bbva")
		os.Exit(1)
	}

	probes := getProbes(*bankCode)
	pages := getPages(*bankCode)

	fmt.Println("================================================================")
	fmt.Printf("  IFRAME DISCOVERY: %s\n", strings.ToUpper(*bankCode))
	fmt.Println("================================================================")
	fmt.Println()
	fmt.Println("This tool inspects the iframe tree on each page and reports")
	fmt.Println("which frame contains which selectors.")
	fmt.Println()

	// Launch visible browser
	url := launcher.New().
		Bin("/usr/bin/google-chrome").
		Headless(false).
		Set("disable-blink-features", "AutomationControlled").
		Set("exclude-switches", "enable-automation").
		Set("no-first-run").
		Set("no-default-browser-check").
		Set("window-size", "1920,1080").
		Devtools(false).
		MustLaunch()

	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser)

	reader := bufio.NewReader(os.Stdin)

	for _, pg := range pages {
		fmt.Println("----------------------------------------------------------------")
		fmt.Printf("PAGE: %s\n", pg.Name)
		fmt.Printf("  -> %s\n", pg.Instructions)
		fmt.Print("  Press ENTER when ready (or 'skip'/'quit'): ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "quit" {
			break
		}
		if input == "skip" {
			fmt.Printf("  Skipped.\n\n")
			continue
		}

		// Wait for DOM stability across all frames
		browserutil.WaitForIFrames(page)
		time.Sleep(500 * time.Millisecond)

		pageURL := page.MustInfo().URL
		fmt.Printf("\n  URL: %s\n\n", pageURL)

		inspectFrame(page, "main", 1, probes)
		fmt.Println()
	}

	fmt.Println("================================================================")
	fmt.Println("  Discovery complete.")
	fmt.Println("  Copy the output above into docs/bbva-webpage-behavior.md")
	fmt.Println("  under the '## Iframe Map' section.")
	fmt.Println("================================================================")
}

// inspectFrame recursively inspects a frame for known selectors and child iframes.
func inspectFrame(page *rod.Page, path string, depth int, probes []selectorProbe) {
	indent := strings.Repeat("  ", depth)

	// Probe for known selectors in this frame
	found := 0
	for _, probe := range probes {
		el, err := page.Timeout(500 * time.Millisecond).Element(probe.Selector)
		if err != nil {
			continue
		}
		visible, _ := el.Visible()
		tag, _ := el.Eval(`() => this.tagName.toLowerCase()`)
		tagName := ""
		if tag != nil {
			tagName = tag.Value.Str()
		}
		fmt.Printf("%sFOUND  %-30s  %s  (visible=%v, tag=%s)\n",
			indent, probe.Name, probe.Selector, visible, tagName)
		found++
	}
	if found == 0 {
		fmt.Printf("%s(no known selectors found)\n", indent)
	}

	// Find child iframes
	iframes, err := page.Elements("iframe")
	if err != nil {
		return
	}

	for i, iframe := range iframes {
		src, _ := iframe.Attribute("src")
		id, _ := iframe.Attribute("id")
		name, _ := iframe.Attribute("name")
		visible, _ := iframe.Visible()

		srcStr := deref(src)
		idStr := deref(id)
		nameStr := deref(name)

		// Build a readable identifier for this iframe
		label := fmt.Sprintf("iframe[%d]", i)
		if idStr != "" {
			label = fmt.Sprintf("iframe#%s", idStr)
		} else if nameStr != "" {
			label = fmt.Sprintf("iframe[name=%s]", nameStr)
		}

		childPath := fmt.Sprintf("%s > %s", path, label)

		fmt.Printf("\n%sIFRAME %s  visible=%v  src=%s\n", indent, childPath, visible, truncate(srcStr, 80))

		frame, err := iframe.Frame()
		if err != nil {
			fmt.Printf("%s  (cannot access frame: %v)\n", indent, err)
			continue
		}

		inspectFrame(frame, childPath, depth+1, probes)
	}
}

func getProbes(bankCode string) []selectorProbe {
	switch bankCode {
	case "bbva":
		return bbvaProbes
	default:
		fmt.Printf("No probes defined for bank %q, using BBVA defaults\n", bankCode)
		return bbvaProbes
	}
}

func getPages(bankCode string) []pageToInspect {
	switch bankCode {
	case "bbva":
		return bbvaPages
	default:
		return bbvaPages
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
