// Package web embeds the built single-page app (web/dist) into the binary.
//
// web/dist is produced by `pnpm -C web build`. A placeholder .gitkeep keeps the
// directory present so the Go build works before the frontend is built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built SPA file system rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // unreachable: "dist" is embedded above
	}
	return sub
}
