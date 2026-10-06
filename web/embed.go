// Package web holds the page gloss serves when it is shown in a browser.
package web

import "embed"

// Page is the viewer's page and the scripts it loads beside Booba's.
//
//go:embed serve.html serve.mjs drop.mjs upload.mjs pickers.mjs remote.mjs pick.html pick.mjs pick-api.mjs
var Page embed.FS
