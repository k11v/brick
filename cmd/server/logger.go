package main

import (
	"io"
	"log/slog"
)

// newLogger returns a new slog.Logger.
func newLogger(w io.Writer, development bool) *slog.Logger {
	var handler slog.Handler
	if development {
		opts := &slog.HandlerOptions{Level: slog.LevelDebug}
		handler = slog.NewTextHandler(w, opts)
	} else {
		opts := &slog.HandlerOptions{Level: slog.LevelInfo}
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}
