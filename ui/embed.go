// Package ui embeds the compiled SvelteKit static bundle for single-binary
// deployment.  The build/ directory is populated by `pnpm build` (or
// `make ui-build`) before `go build`.  An empty build/ stub is kept in the
// repo so `go build` succeeds even before the first UI build.
package ui

import "embed"

//go:embed build
var StaticFiles embed.FS
