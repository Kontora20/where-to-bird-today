// Package web embeds the built SvelteKit SPA so cmd/server can serve it
// without a separate static-file deployment step.
//
// This file has to live here, not under cmd/ or internal/: //go:embed
// patterns cannot use ../ to reach outside the directory containing the
// directive, and web/ is a sibling of cmd/ and internal/, not a subdirectory
// of either.
package web

import "embed"

// all: is required, not decoration -- SvelteKit's static build emits a
// leading-underscore _app/ directory for its JS chunks, and a plain
// "//go:embed build" silently excludes any file or directory starting with
// _ or ., losing every chunk with no compile error, just a broken page at
// runtime.
//
//go:embed all:build
var BuildFS embed.FS
