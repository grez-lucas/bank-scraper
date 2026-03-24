// Package static embeds static assets (images, etc.) for the credential manager web UI.
package static

import "embed"

//go:embed *.png

// FS holds the embedded static assets (images) for the credential manager web UI.
var FS embed.FS
