// Package debug provides centralized artifact capture and operation logging
// for the bank scraper. All Collector methods are nil-receiver safe (no-ops)
// and never return errors — debug must never fail the operation.
package debug

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/go-rod/rod"
)

// Collector writes debug artifacts to a session-scoped directory.
// All methods are nil-receiver safe (no-ops). Capture errors are logged
// at Debug level but never returned — debug must never fail the operation.
type Collector struct {
	dir    string
	logger *slog.Logger
}

// New creates a Collector that writes artifacts under baseDir/sessionID/.
// The directory is created lazily on first write, not at construction time.
func New(baseDir, sessionID string, logger *slog.Logger) *Collector {
	return &Collector{
		dir:    filepath.Join(baseDir, sessionID),
		logger: logger,
	}
}

// Dir returns the artifact directory path.
func (c *Collector) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

// Screenshot captures a full-page screenshot and writes it to disk.
func (c *Collector) Screenshot(page *rod.Page, operation, reason string) {
	if c == nil {
		return
	}
	data, err := page.Screenshot(true, nil)
	if err != nil {
		c.logger.Debug("screenshot capture failed", slog.String("operation", operation), slog.Any("error", err))
		return
	}
	if len(data) == 0 {
		return
	}
	path := c.artifactPath(operation, reason, ".png")
	c.writeFile(path, data)
}

// HTML captures the page's full HTML and writes it to disk.
func (c *Collector) HTML(page *rod.Page, operation, reason string) {
	if c == nil {
		return
	}
	html, err := page.HTML()
	if err != nil {
		c.logger.Debug("HTML capture failed", slog.String("operation", operation), slog.Any("error", err))
		return
	}
	if len(html) == 0 {
		return
	}
	path := c.artifactPath(operation, reason, ".html")
	c.writeFile(path, []byte(html))
}

// HTMLString writes an HTML string to disk.
func (c *Collector) HTMLString(html, operation, reason string) {
	if c == nil {
		return
	}
	if len(html) == 0 {
		return
	}
	path := c.artifactPath(operation, reason, ".html")
	c.writeFile(path, []byte(html))
}

// JSON writes a JSON string to disk.
func (c *Collector) JSON(data, operation, reason string) {
	if c == nil {
		return
	}
	if len(data) == 0 {
		return
	}
	path := c.artifactPath(operation, reason, ".json")
	c.writeFile(path, []byte(data))
}

// Snapshot captures both a screenshot and the page HTML, plus the current URL.
// Returns the page URL and the artifact directory. PageURL is fetched even on
// nil receiver so callers can include it in error messages without a separate call.
func (c *Collector) Snapshot(page *rod.Page, operation, reason string) (pageURL, dir string) {
	url := PageURL(page)
	if c == nil {
		return url, ""
	}
	c.Screenshot(page, operation, reason)
	c.HTML(page, operation, reason)
	return url, c.dir
}

// PageURL returns the current page URL, or empty string on error.
func PageURL(page *rod.Page) string {
	if page == nil {
		return ""
	}
	info, err := page.Info()
	if err != nil || info == nil {
		return ""
	}
	return info.URL
}

func (c *Collector) artifactPath(operation, reason, ext string) string {
	name := fmt.Sprintf("%s-%s%s", operation, reason, ext)
	return filepath.Join(c.dir, name)
}

func (c *Collector) ensureDir() {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		c.logger.Debug("failed to create debug dir", slog.String("dir", c.dir), slog.Any("error", err))
	}
}

func (c *Collector) writeFile(path string, data []byte) {
	c.ensureDir()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		c.logger.Debug("failed to write debug artifact",
			slog.String("path", path),
			slog.Any("error", err))
	}
}
