// Package templates embeds the HTML template files for the credential manager web UI.
package templates

import "embed"

//go:embed *.html

// FS holds the embedded HTML templates for the credential manager web UI.
var FS embed.FS
