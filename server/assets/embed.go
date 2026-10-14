// Package assets contains static files bundled into the server binary.
package assets

import "embed"

// HTML contains the public HTML pages served by the application.
//
//go:embed html/*.html
var HTML embed.FS
