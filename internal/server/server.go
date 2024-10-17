package server

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// New returns a new HTTP server.
// It should be started with http.Server's ListenAndServe.
func New(cfg *Config, log *slog.Logger, _ *pgxpool.Pool) *http.Server {
	addr := net.JoinHostPort(cfg.host(), strconv.Itoa(cfg.port()))

	subLogger := log.With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	mux := http.NewServeMux()

	// TODO: Don't register in production.
	mux.Handle("GET /swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}
}
