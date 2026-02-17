package testutil

import (
	"net/url"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// SensitivePatterns contains regex patterns for sensitive data.
var SensitivePatterns = []string{
	// Password fields
	`(?i)password`,
	`(?i)passwd`,
	`(?i)clave`,
	`(?i)contrase`,
	`(?i)secret`,

	// Tokens and sessions
	`(?i)token`,
	`(?i)session`,
	`(?i)sess_`,
	`(?i)auth`,
	`(?i)jwt`,
	`(?i)bearer`,

	// API keys
	`(?i)api_?key`,
	`(?i)apikey`,

	// Credentials
	`(?i)credential`,
	`(?i)access_key`,
	`(?i)private_key`,
}

// SensitiveHeaders are headers that should be redacted.
var SensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-auth-token":        true,
	"x-api-key":           true,
	"x-access-token":      true,
	"x-session-id":        true,
	"x-csrf-token":        true,
	"x-xsrf-token":        true,
	"proxy-authorization": true,
}

// SanitizeHAR redacts sensitive data from a HAR log.
// Returns a new HARLog with sensitive data replaced by [REDACTED].
func SanitizeHAR(har *HARLog) *HARLog {
	sanitized := &HARLog{
		Entries: make([]HAREntry, len(har.Entries)),
	}

	for i, entry := range har.Entries {
		sanitized.Entries[i] = sanitizeEntry(entry)
	}

	return sanitized
}

func sanitizeEntry(entry HAREntry) HAREntry {
	return HAREntry{
		Request:  sanitizeRequest(entry.Request),
		Response: sanitizeResponse(entry.Response),
	}
}

func sanitizeRequest(req HARRequest) HARRequest {
	return HARRequest{
		Method:  req.Method,
		URL:     sanitizeURL(req.URL),
		Headers: sanitizeHeaders(req.Headers),
		Body:    sanitizeBody(req.Body),
	}
}

func sanitizeResponse(resp HARResponse) HARResponse {
	return HARResponse{
		Status:  resp.Status,
		Headers: sanitizeHeaders(resp.Headers),
		Content: HARContent{
			MimeType: resp.Content.MimeType,
			Text:     sanitizeBody(resp.Content.Text),
			Encoding: resp.Content.Encoding,
			Size:     resp.Content.Size,
		},
	}
}

func sanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Sanitize query parameters
	query := parsed.Query()
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, redacted)
		}
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func sanitizeHeaders(headers []HARHeader) []HARHeader {
	sanitized := make([]HARHeader, len(headers))

	for i, h := range headers {
		name := strings.ToLower(h.Name)
		if SensitiveHeaders[name] {
			sanitized[i] = HARHeader{
				Name:  h.Name,
				Value: redacted,
			}
		} else if isSensitiveKey(h.Name) {
			sanitized[i] = HARHeader{
				Name:  h.Name,
				Value: redacted,
			}
		} else {
			sanitized[i] = h
		}
	}

	return sanitized
}

func sanitizeBody(body string) string {
	if body == "" {
		return body
	}

	result := body

	// Sanitize form-encoded bodies (key=value&key2=value2)
	if strings.Contains(body, "=") && !strings.HasPrefix(body, "{") {
		result = sanitizeFormBody(result)
	}

	// Sanitize JSON bodies
	if strings.HasPrefix(strings.TrimSpace(body), "{") || strings.HasPrefix(strings.TrimSpace(body), "[") {
		result = sanitizeJSONBody(result)
	}

	return result
}

func sanitizeFormBody(body string) string {
	values, err := url.ParseQuery(body)
	if err != nil {
		return body
	}

	for key := range values {
		if isSensitiveKey(key) {
			values.Set(key, redacted)
		}
	}

	return values.Encode()
}

func sanitizeJSONBody(body string) string {
	result := body

	// Use regex to find and redact sensitive JSON fields
	// Pattern: "sensitive_key": "value" or "sensitive_key":"value"
	for _, pattern := range SensitivePatterns {
		re := regexp.MustCompile(`("` + pattern + `")\s*:\s*"[^"]*"`)
		result = re.ReplaceAllString(result, `$1: "`+redacted+`"`)

		// Also handle non-string values
		re = regexp.MustCompile(`("` + pattern + `")\s*:\s*([^",}\]]+)`)
		result = re.ReplaceAllString(result, `$1: "`+redacted+`"`)
	}

	return result
}

func isSensitiveKey(key string) bool {
	keyLower := strings.ToLower(key)

	for _, pattern := range SensitivePatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(keyLower) {
			return true
		}
	}

	return false
}
