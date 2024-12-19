package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	server := newServer(&config{})

	slog.Info("starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
