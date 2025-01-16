//go:build dev

package main

import (
	"os"
	"path/filepath"
	"runtime"
)

func init() {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("didn't get caller")
	}
	packageDir := filepath.Dir(sourceFile)
	dataDir := filepath.Join(packageDir, "data")
	dataFS = os.DirFS(dataDir)
}
