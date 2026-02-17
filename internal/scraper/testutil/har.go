// Package testutil provides testing utilities for the scraper package,
// including HAR recording and replay capabilities.
package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// HARLog represents a simplified HAR (HTTP Archive) format for recording
// and replaying browser sessions.
type HARLog struct {
	Entries []HAREntry `json:"entries"`
}

// HAREntry represents a single HTTP request/response pair.
type HAREntry struct {
	Request  HARRequest  `json:"request"`
	Response HARResponse `json:"response"`
}

// HARRequest represents an HTTP request.
type HARRequest struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers []HARHeader `json:"headers,omitempty"`
	Body    string      `json:"body,omitempty"`
}

// HARResponse represents an HTTP response.
type HARResponse struct {
	Status  int         `json:"status"`
	Headers []HARHeader `json:"headers,omitempty"`
	Content HARContent  `json:"content"`
}

// HARHeader represents an HTTP header key-value pair.
type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARContent represents the response body content.
type HARContent struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`               // Plain text or base64 encoded
	Encoding string `json:"encoding,omitempty"` // "base64" if binary content
	Size     int    `json:"size,omitempty"`
}

// ============================================================================
// Chrome DevTools HAR 1.2 Format Support
// ============================================================================

// ChromeHAR represents the full HAR 1.2 format exported by Chrome DevTools.
// Chrome wraps entries in a "log" object and uses postData instead of body.
type ChromeHAR struct {
	Log ChromeHARLog `json:"log"`
}

// ChromeHARLog is the log wrapper in Chrome's HAR format.
type ChromeHARLog struct {
	Version string           `json:"version"`
	Creator ChromeHARCreator `json:"creator"`
	Entries []ChromeHAREntry `json:"entries"`
}

// ChromeHARCreator identifies the tool that created the HAR.
type ChromeHARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ChromeHAREntry represents a single request/response in Chrome's format.
type ChromeHAREntry struct {
	Request  ChromeHARRequest  `json:"request"`
	Response ChromeHARResponse `json:"response"`
}

// ChromeHARRequest represents an HTTP request in Chrome's format.
type ChromeHARRequest struct {
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Headers  []HARHeader  `json:"headers,omitempty"`
	PostData *HARPostData `json:"postData,omitempty"`
}

// HARPostData represents POST body data in Chrome's HAR format.
type HARPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// ChromeHARResponse represents an HTTP response in Chrome's format.
type ChromeHARResponse struct {
	Status  int         `json:"status"`
	Headers []HARHeader `json:"headers,omitempty"`
	Content HARContent  `json:"content"`
}

// LoadHAR reads a HAR file from the given path.
// It auto-detects Chrome DevTools HAR 1.2 format (with "log" wrapper)
// and converts it to the simplified format used internally.
func LoadHAR(path string) (*HARLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read HAR file: %w", err)
	}

	// Try Chrome DevTools format first (has "log" wrapper)
	var chromeHAR ChromeHAR
	if err := json.Unmarshal(data, &chromeHAR); err == nil && len(chromeHAR.Log.Entries) > 0 {
		return convertChromeHAR(&chromeHAR), nil
	}

	// Fall back to simplified format
	var har HARLog
	if err := json.Unmarshal(data, &har); err != nil {
		return nil, fmt.Errorf("parse HAR JSON: %w", err)
	}

	return &har, nil
}

// convertChromeHAR converts Chrome DevTools HAR format to our simplified format.
func convertChromeHAR(chrome *ChromeHAR) *HARLog {
	entries := make([]HAREntry, len(chrome.Log.Entries))

	for i, ce := range chrome.Log.Entries {
		// Convert request, mapping postData.text to body
		var body string
		if ce.Request.PostData != nil {
			body = ce.Request.PostData.Text
		}

		entries[i] = HAREntry{
			Request: HARRequest{
				Method:  ce.Request.Method,
				URL:     ce.Request.URL,
				Headers: ce.Request.Headers,
				Body:    body,
			},
			Response: HARResponse{
				Status:  ce.Response.Status,
				Headers: ce.Response.Headers,
				Content: ce.Response.Content,
			},
		}
	}

	return &HARLog{Entries: entries}
}

// SaveHAR writes a HAR log to the given path with pretty formatting.
func SaveHAR(path string, har *HARLog) error {
	data, err := json.MarshalIndent(har, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal HAR: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write HAR file: %w", err)
	}

	return nil
}

// MustLoadHAR loads a HAR file and fails the test if it cannot be loaded.
func MustLoadHAR(t *testing.T, path string) *HARLog {
	t.Helper()

	har, err := LoadHAR(path)
	if err != nil {
		t.Fatalf("failed to load HAR file %s: %v", path, err)
	}

	return har
}
