// Package ui embeds the built web frontend. The Vite build writes to
// internal/ui/dist; the committed placeholder keeps `go build` working
// before the frontend has been built.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the frontend files rooted at the build output directory.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees dist exists
	}
	return sub
}
