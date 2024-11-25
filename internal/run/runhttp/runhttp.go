package runhttp

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/k11v/brick/internal/app/apphttp"
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

func NewServer(conf *Config) *http.Server {
	addr := net.JoinHostPort(conf.host(), strconv.Itoa(conf.port()))

	subLogger := slog.Default().With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	appHandler := &apphttp.Handler{}

	mux := &http.ServeMux{}
	stubHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	mux.Handle("GET /swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json"))) // TODO: don't register in production
	mux.Handle("POST /builds", stubHandler)
	mux.Handle("POST /builds/{id}/cancel", stubHandler)
	mux.Handle("GET /builds", stubHandler)
	mux.Handle("GET /builds/{id}", stubHandler)
	mux.Handle("GET /builds/{id}/wait", stubHandler)
	mux.Handle("GET /builds/limits", stubHandler)
	mux.HandleFunc("GET /health", appHandler.GetHealth)

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
	}
}
