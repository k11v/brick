package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/k11v/brick/internal/run/runhttp"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	server := runhttp.NewServer(&runhttp.Config{})

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
