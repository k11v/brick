package http

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type Config struct {
	Host              string // default: "127.0.0.1"
	Port              int    // default: 8080
	ReadHeaderTimeout time.Duration
}

func (cfg *Config) host() string {
	h := cfg.Host
	if h == "" {
		h = "127.0.0.1"
	}
	return h
}

func (cfg *Config) port() int {
	p := cfg.Port
	if p == 0 {
		p = 8080
	}
	return p
}

func NewServer(cfg *Config) *http.Server {
	addr := net.JoinHostPort(cfg.host(), strconv.Itoa(cfg.port()))

	subLogger := slog.Default().With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	mux := &http.ServeMux{}
	stubHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	mux.Handle("GET /swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json"))) // TODO: don't register in production
	mux.Handle("POST /builds", stubHandler)
	mux.Handle("POST /builds/{id}/cancel", stubHandler)
	mux.Handle("GET /builds", stubHandler)
	mux.Handle("GET /builds/{id}", stubHandler)
	mux.Handle("GET /builds/{id}/wait", stubHandler)
	mux.Handle("GET /builds/limits", stubHandler)
	mux.Handle("GET /health", stubHandler)

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}
}
