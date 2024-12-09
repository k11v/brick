package static

import "embed"

//go:embed error.tmpl favicon.ico main.tmpl
var FS embed.FS
