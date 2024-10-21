package server

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New returns a new HTTP server.
// It should be started with http.Server's ListenAndServe.
func New(cfg *Config, log *slog.Logger, _ *pgxpool.Pool) *http.Server {
	addr := net.JoinHostPort(cfg.host(), strconv.Itoa(cfg.port()))

	subLogger := log.With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	h := newHandler()

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           h,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}
}
