//go:build !dev

package main

import (
	"embed"
	"io/fs"
)

//go:embed data
var embedDataFS embed.FS

func init() {
	var err error
	dataFS, err = fs.Sub(embedDataFS, "data")
	if err != nil {
		panic(err)
	}
}