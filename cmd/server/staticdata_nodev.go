//go:build !dev

package main

import (
	"embed"
	"io/fs"
)

//go:embed staticdata
var embedStaticFS embed.FS

func init() {
	var err error
	staticFS, err = fs.Sub(embedStaticFS, "staticdata")
	if err != nil {
		panic(err)
	}
}
