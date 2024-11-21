package server

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type handler struct {
	mux *http.ServeMux
}

func newHandler() *handler {
	mux := http.NewServeMux()
	h := &handler{mux: mux}

	// TODO: Don't register in production.
	mux.Handle("GET /swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))

	// mux.HandleFunc("GET /health", h.GetHealth)

	// mux.HandleFunc("POST /builds", h.CreateBuild)
	// mux.HandleFunc("GET /builds/{id}", h.GetBuild)
	// mux.HandleFunc("GET /builds", h.ListBuilds)
	// mux.HandleFunc("POST /builds/{id}/cancel", h.CancelBuild)
	// mux.HandleFunc("GET /builds/{id}/wait", h.WaitForBuild)

	// mux.HandleFunc("GET /builds/limits", h.GetLimits)

	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
