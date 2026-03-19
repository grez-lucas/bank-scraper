// Package templates embeds the HTML template files for the credential manager web UI.
package templates

import "embed"

//go:embed *.html
var FS embed.FS
