//go:build !embedweb

package web

import "io/fs"

// Assets returns nil in development; use the Vite dev server or BURROW_STATIC_DIR.
func Assets() fs.FS { return nil }
