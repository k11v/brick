package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	worker := &Worker{}

	slog.Info("starting worker")
	if err := worker.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
