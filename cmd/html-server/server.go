package main

import (
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/k11v/brick/cmd/html-server/static"
)

var (
	funcs = template.FuncMap{}
	fs    = static.FS
)

func newServer(conf *config) *http.Server {
	addr := net.JoinHostPort(conf.host(), strconv.Itoa(conf.port()))

	subLogger := logger.With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	mux := &http.ServeMux{}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := writePage(w, nil, "main.tmpl")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeErrorPage(w, http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := writeErrorPage(w, http.StatusNotFound)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeErrorPage(w, http.StatusInternalServerError)
		}
	})

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
	}
}

// writePage.
// The first template name is the one being executed.
func writePage(w io.Writer, data any, templateNames ...string) error {
	tmpl := template.Must(template.New(templateNames[0]).Funcs(funcs).ParseFS(fs, templateNames...))
	if err := tmpl.Execute(w, data); err != nil {
		return err
	}
	return nil
}

func writeErrorPage(w io.Writer, statusCode int) error {
	return writePage(w, statusCode, "error.tmpl")
}
