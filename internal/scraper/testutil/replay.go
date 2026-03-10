package testutil

import (
	"encoding/base64"
	"log"
	"net/url"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Replayer serves recorded HTTP responses during test execution.
type Replayer struct {
	// exactMatches maps full URLs to entries (last wins)
	exactMatches map[string]*HAREntry

	// pathMatches maps URL paths (without query params) to entries (first wins)
	pathMatches map[string]*HAREntry

	// methodExact maps "METHOD|URL" to entries (last wins per method+URL).
	// Used to disambiguate requests to the same URL with different methods
	// (e.g., POST and DELETE to grantingTicket/V02 in the Senda login flow).
	methodExact map[string]*HAREntry

	// methodPath maps "METHOD|path" to entries (first wins per method+path)
	methodPath map[string]*HAREntry

	// passthrough allows unmatched requests to go to the network
	passthrough bool

	// verbose enables logging of matched/unmatched requests
	verbose bool
}

// ReplayerOption configures a Replayer.
type ReplayerOption func(*Replayer)

// WithPassthrough allows unmatched requests to go to the real network.
// By default, unmatched requests will fail.
func WithPassthrough(enabled bool) ReplayerOption {
	return func(r *Replayer) {
		r.passthrough = enabled
	}
}

// WithVerbose enables verbose logging of request matching.
func WithVerbose(enabled bool) ReplayerOption {
	return func(r *Replayer) {
		r.verbose = enabled
	}
}

// NewReplayer creates a replayer from a HAR log.
func NewReplayer(har *HARLog, opts ...ReplayerOption) *Replayer {
	r := &Replayer{
		exactMatches: make(map[string]*HAREntry),
		pathMatches:  make(map[string]*HAREntry),
		methodExact:  make(map[string]*HAREntry),
		methodPath:   make(map[string]*HAREntry),
		passthrough:  false,
		verbose:      false,
	}

	for _, opt := range opts {
		opt(r)
	}

	// Index entries for fast lookup
	for i := range har.Entries {
		entry := &har.Entries[i]
		reqURL := entry.Request.URL

		// Store exact match (last wins)
		r.exactMatches[reqURL] = entry

		// Store method+exact match (last wins per method+URL)
		methodKey := entry.Request.Method + "|" + reqURL
		r.methodExact[methodKey] = entry

		// Store path-only match (fallback)
		if parsed, err := url.Parse(reqURL); err == nil {
			pathKey := parsed.Scheme + "://" + parsed.Host + parsed.Path

			// Only store first occurrence for path matches
			if _, exists := r.pathMatches[pathKey]; !exists {
				r.pathMatches[pathKey] = entry
			}

			// Store method+path match (first wins per method+path)
			methodPathKey := entry.Request.Method + "|" + pathKey
			if _, exists := r.methodPath[methodPathKey]; !exists {
				r.methodPath[methodPathKey] = entry
			}
		}
	}

	return r
}

// Middleware returns a Rod hijack handler that serves recorded responses.
// Use with router.MustAdd("*", replayer.Middleware()).
func (r *Replayer) Middleware() func(*rod.Hijack) {
	return func(ctx *rod.Hijack) {
		reqURL := ctx.Request.URL().String()
		method := ctx.Request.Method()

		// 1. Method + exact URL (best match for same-URL-different-method cases)
		methodKey := method + "|" + reqURL
		entry, found := r.methodExact[methodKey]

		// 2. Method + path only
		if !found {
			if parsed, err := url.Parse(reqURL); err == nil {
				pathKey := parsed.Scheme + "://" + parsed.Host + parsed.Path
				methodPathKey := method + "|" + pathKey
				entry, found = r.methodPath[methodPathKey]
			}
		}

		// 3. Exact URL (fallback, ignores method)
		if !found {
			entry, found = r.exactMatches[reqURL]
		}

		// 4. Path only (fallback, ignores method)
		if !found {
			if parsed, err := url.Parse(reqURL); err == nil {
				pathKey := parsed.Scheme + "://" + parsed.Host + parsed.Path
				entry, found = r.pathMatches[pathKey]
			}
		}

		if !found {
			if r.verbose {
				log.Printf("[replayer] no match for: %s %s", method, reqURL)
			}

			if r.passthrough {
				// Let it go to the real network
				_ = ctx.LoadResponse(nil, true)
				return
			}

			// Fail with 404 for unmatched requests
			r.serveNotFound(ctx, reqURL)
			return
		}

		if r.verbose {
			log.Printf("[replayer] matched: %s %s -> %d", method, reqURL, entry.Response.Status)
		}

		r.serveRecordedResponse(ctx, entry)
	}
}

// serveRecordedResponse serves a recorded HAR entry as the response.
// For 3xx redirects, it follows the redirect chain and returns the final response.
func (r *Replayer) serveRecordedResponse(ctx *rod.Hijack, entry *HAREntry) {
	// Follow redirect chain if this is a 3xx response
	finalEntry := r.followRedirects(entry)
	resp := finalEntry.Response

	// Decode body if base64 encoded
	var body []byte
	if resp.Content.Encoding == "base64" {
		var err error
		body, err = base64.StdEncoding.DecodeString(resp.Content.Text)
		if err != nil {
			body = []byte(resp.Content.Text)
		}
	} else {
		body = []byte(resp.Content.Text)
	}

	// Build response headers for the protocol
	var protoHeaders []*proto.FetchHeaderEntry
	for _, h := range resp.Headers {
		name := strings.ToLower(h.Name)
		// Skip problematic headers
		if name == "content-encoding" || name == "content-length" || name == "location" {
			continue
		}
		protoHeaders = append(protoHeaders, &proto.FetchHeaderEntry{
			Name:  h.Name,
			Value: h.Value,
		})
	}

	// Add content type if not present
	hasContentType := false
	for _, h := range protoHeaders {
		if strings.ToLower(h.Name) == "content-type" {
			hasContentType = true
			break
		}
	}
	if !hasContentType && resp.Content.MimeType != "" {
		protoHeaders = append(protoHeaders, &proto.FetchHeaderEntry{
			Name:  "Content-Type",
			Value: resp.Content.MimeType,
		})
	}

	// Set up the response payload directly
	payload := ctx.Response.Payload()
	payload.ResponseCode = resp.Status
	payload.ResponseHeaders = protoHeaders
	payload.Body = body
}

// followRedirects follows a redirect chain and returns the final entry.
// If the entry is not a redirect or the target is not in the HAR, returns the original entry.
func (r *Replayer) followRedirects(entry *HAREntry) *HAREntry {
	const maxRedirects = 10
	current := entry

	for i := 0; i < maxRedirects; i++ {
		// Check if this is a redirect (3xx status)
		if current.Response.Status < 300 || current.Response.Status >= 400 {
			return current
		}

		// Find the Location header
		var location string
		for _, h := range current.Response.Headers {
			if strings.ToLower(h.Name) == "location" {
				location = h.Value
				break
			}
		}

		if location == "" {
			return current
		}

		if r.verbose {
			log.Printf("[replayer] following redirect: %d -> %s", current.Response.Status, location)
		}

		// Look up the redirect target
		target, found := r.exactMatches[location]
		if !found {
			// Try path-only match
			if parsed, err := url.Parse(location); err == nil {
				pathKey := parsed.Scheme + "://" + parsed.Host + parsed.Path
				target, found = r.pathMatches[pathKey]
			}
		}

		if !found {
			if r.verbose {
				log.Printf("[replayer] redirect target not in HAR: %s", location)
			}
			return current
		}

		current = target
	}

	return current
}

// serveNotFound serves a 404 response for unmatched requests.
func (r *Replayer) serveNotFound(ctx *rod.Hijack, reqURL string) {
	body := []byte(`{"error": "no recording found for URL"}`)

	payload := ctx.Response.Payload()
	payload.ResponseCode = 404
	payload.ResponseHeaders = []*proto.FetchHeaderEntry{
		{Name: "Content-Type", Value: "application/json"},
	}
	payload.Body = body

	if r.verbose {
		log.Printf("[replayer] 404 not found: %s", reqURL)
	}
}

// Stats returns statistics about the replayer's index.
func (r *Replayer) Stats() map[string]int {
	return map[string]int{
		"exact_matches":  len(r.exactMatches),
		"path_matches":   len(r.pathMatches),
		"method_exact":   len(r.methodExact),
		"method_path":    len(r.methodPath),
	}
}
