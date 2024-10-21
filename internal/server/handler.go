package server

import (
	"encoding/json"
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

	mux.HandleFunc("GET /health", h.GetHealth)

	mux.HandleFunc("POST /builds", h.CreateBuild)
	mux.HandleFunc("GET /builds/{id}", h.GetBuild)
	mux.HandleFunc("GET /builds", h.ListBuilds)
	mux.HandleFunc("POST /builds/{id}/cancel", h.CancelBuild)
	mux.HandleFunc("POST /builds/{id}/wait", h.WaitForBuild)

	mux.HandleFunc("GET /builds/limits", h.GetLimits)

	mux.HandleFunc("DELETE /builds/caches/{key}", h.DeleteCache)

	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Status string `json:"status"`
	}

	resp := response{Status: "ok"}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *handler) CreateBuild(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) GetBuild(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) ListBuilds(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) CancelBuild(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) WaitForBuild(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) GetLimits(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) DeleteCache(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
