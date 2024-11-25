package runhttp

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/k11v/brick/internal/app/apphttp"
	"github.com/k11v/brick/internal/buildtask/buildtaskhttp"
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

func NewServer(conf *Config, development bool) *http.Server {
	addr := net.JoinHostPort(conf.host(), strconv.Itoa(conf.port()))

	subLogger := slog.Default().With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	appHandler := &apphttp.Handler{}
	buildtaskHandler := &buildtaskhttp.Handler{}

	mux := &http.ServeMux{}
	mux.HandleFunc("POST /builds", buildtaskHandler.CreateBuild)
	mux.HandleFunc("POST /builds/{id}/cancel", buildtaskHandler.CancelBuild)
	mux.HandleFunc("GET /builds", buildtaskHandler.ListBuilds)
	mux.HandleFunc("GET /builds/{id}", buildtaskHandler.GetBuild)
	mux.HandleFunc("GET /builds/{id}/wait", buildtaskHandler.WaitForBuild)
	mux.HandleFunc("GET /builds/limits", buildtaskHandler.GetLimits)
	mux.HandleFunc("GET /health", appHandler.GetHealth)
	if development {
		mux.Handle("GET /swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
	}
}
