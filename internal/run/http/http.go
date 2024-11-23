package http

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"
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

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}
}
