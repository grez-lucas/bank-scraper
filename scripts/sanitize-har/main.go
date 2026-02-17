// sanitize-har removes sensitive data from HAR files before committing.
//
// Usage:
//
//	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login_success
//	go run ./scripts/sanitize-har/main.go -input=recording.har.json -output=sanitized.har.json
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grez-lucas/bank-scraper/internal/scraper/testutil"
)

func main() {
	// Conventional path flags
	bankCode := flag.String("bank", "", "Bank code: bbva, interbank, bcp")
	scenario := flag.String("scenario", "", "Scenario name (e.g., login_success)")

	// Direct path flags
	inputPath := flag.String("input", "", "Input HAR file path")
	outputPath := flag.String("output", "", "Output HAR file path (defaults to input path)")

	// Options
	dryRun := flag.Bool("dry-run", false, "Show what would be redacted without modifying")

	flag.Parse()

	// Determine input/output paths
	var inPath, outPath string

	if *bankCode != "" && *scenario != "" {
		// Conventional path: internal/scraper/bank/{bank}/testdata/recordings/{scenario}.har.json
		inPath = filepath.Join("internal", "scraper", "bank", *bankCode, "testdata", "recordings", *scenario+".har.json")
		outPath = inPath // Sanitize in place
	} else if *inputPath != "" {
		inPath = *inputPath
		outPath = *inputPath
		if *outputPath != "" {
			outPath = *outputPath
		}
	} else {
		printUsage()
		os.Exit(1)
	}

	// Check input file exists
	if _, err := os.Stat(inPath); os.IsNotExist(err) {
		fmt.Printf("Error: Input file not found: %s\n", inPath)
		os.Exit(1)
	}

	fmt.Printf("Loading HAR file: %s\n", inPath)

	// Load HAR (auto-detects Chrome vs simplified format)
	har, err := testutil.LoadHAR(inPath)
	if err != nil {
		fmt.Printf("Error loading HAR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d entries\n", len(har.Entries))

	// Sanitize
	sanitized := testutil.SanitizeHAR(har)

	// Count redactions
	redactionCount := countRedactions(har, sanitized)
	fmt.Printf("Redacted %d sensitive values\n", redactionCount)

	if *dryRun {
		fmt.Println("\n[DRY RUN] No changes written.")
		printRedactionSummary(har, sanitized)
		return
	}

	// Save sanitized HAR
	if err := testutil.SaveHAR(outPath, sanitized); err != nil {
		fmt.Printf("Error saving HAR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Sanitized HAR saved to: %s\n", outPath)
	fmt.Println("\nSafe to commit!")
}

func printUsage() {
	fmt.Println("sanitize-har - Remove sensitive data from HAR files before committing")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login_success")
	fmt.Println("  go run ./scripts/sanitize-har/main.go -input=recording.har.json")
	fmt.Println("  go run ./scripts/sanitize-har/main.go -input=in.har.json -output=out.har.json")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -bank      Bank code (bbva, interbank, bcp)")
	fmt.Println("  -scenario  Scenario name (login_success, login_error, etc.)")
	fmt.Println("  -input     Input HAR file path")
	fmt.Println("  -output    Output HAR file path (defaults to input)")
	fmt.Println("  -dry-run   Show redactions without modifying file")
}

func countRedactions(original, sanitized *testutil.HARLog) int {
	count := 0
	for i := range original.Entries {
		if i >= len(sanitized.Entries) {
			break
		}
		orig := original.Entries[i]
		san := sanitized.Entries[i]

		// Compare request URLs
		if orig.Request.URL != san.Request.URL {
			count++
		}

		// Compare request headers
		for j, h := range orig.Request.Headers {
			if j < len(san.Request.Headers) && h.Value != san.Request.Headers[j].Value {
				count++
			}
		}

		// Compare request body
		if orig.Request.Body != san.Request.Body {
			count++
		}

		// Compare response headers
		for j, h := range orig.Response.Headers {
			if j < len(san.Response.Headers) && h.Value != san.Response.Headers[j].Value {
				count++
			}
		}

		// Compare response body
		if orig.Response.Content.Text != san.Response.Content.Text {
			count++
		}
	}
	return count
}

func printRedactionSummary(original, sanitized *testutil.HARLog) {
	fmt.Println("\nRedaction Summary:")
	fmt.Println("==================")

	for i := range original.Entries {
		if i >= len(sanitized.Entries) {
			break
		}
		orig := original.Entries[i]
		san := sanitized.Entries[i]
		hasRedactions := false

		// Check URL
		if orig.Request.URL != san.Request.URL {
			if !hasRedactions {
				fmt.Printf("\nEntry %d: %s %s\n", i+1, orig.Request.Method, truncateURL(orig.Request.URL))
				hasRedactions = true
			}
			fmt.Println("  - URL query parameters redacted")
		}

		// Check headers
		for j, h := range orig.Request.Headers {
			if j < len(san.Request.Headers) && h.Value != san.Request.Headers[j].Value {
				if !hasRedactions {
					fmt.Printf("\nEntry %d: %s %s\n", i+1, orig.Request.Method, truncateURL(orig.Request.URL))
					hasRedactions = true
				}
				fmt.Printf("  - Request header '%s' redacted\n", h.Name)
			}
		}

		// Check body
		if orig.Request.Body != san.Request.Body {
			if !hasRedactions {
				fmt.Printf("\nEntry %d: %s %s\n", i+1, orig.Request.Method, truncateURL(orig.Request.URL))
				hasRedactions = true
			}
			fmt.Println("  - Request body redacted")
		}
	}
}

func truncateURL(url string) string {
	if len(url) > 80 {
		return url[:77] + "..."
	}
	return url
}
