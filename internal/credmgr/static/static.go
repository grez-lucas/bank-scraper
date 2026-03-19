// Package static embeds static assets (images, etc.) for the credential manager web UI.
package static

import "embed"

//go:embed *.png
var FS embed.FS
