package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
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
		return
	}
}

func (h *handler) CreateBuild(w http.ResponseWriter, r *http.Request) {
	type request struct {
		InputFiles *map[string]string `json:"input_files"` // key is path, value is base64-encoded content
		CacheKey   *uuid.UUID         `json:"cache_key"`
	}

	type response struct {
		BuildID uuid.UUID `json:"build_id"`
	}

	// TODO: Make validation a part of parameter preparation for a service function.
	// Validation would be happening parallel to parameter preparation.
	// It would reduce redundant operations (e.g. repeated Base64 decodings).
	// It should cover not only the request body but header and something else as well.
	// It could also include the decoding JSON from r.Body.
	// It should validate header before body to possibly even avoid reading the body.
	// It could have a slice for validation errors that would be populated during preparation.

	// Header

	if len(r.Header.Values("Authorization")) == 0 {
		http.Error(w, "missing request header Authorization", http.StatusUnprocessableEntity)
		return
	}
	if len(r.Header.Values("Authorization")) > 1 {
		http.Error(w, "invalid request header Authorization: multiple values", http.StatusUnprocessableEntity)
		return
	}
	authorizationParts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(authorizationParts) == 0 {
		http.Error(w, "empty request header Authorization", http.StatusUnprocessableEntity)
		return
	}
	if got, want := authorizationParts[0], "Bearer"; strings.ToLower(got) != strings.ToLower(want) {
		http.Error(w, fmt.Errorf("invalid request header Authorization: got scheme %s, want %s", got, want).Error(), http.StatusUnprocessableEntity)
		return
	}
	// Token will be extracted, validated and used to derive userID.

	if len(r.Header.Values("X-Idempotency-Key")) == 0 {
		http.Error(w, "missing request header X-Idempotency-Key", http.StatusUnprocessableEntity)
		return
	}
	if len(r.Header.Values("X-Idempotency-Key")) > 1 {
		http.Error(w, "invalid request header X-Idempotency-Key: multiple values", http.StatusUnprocessableEntity)
		return
	}
	_, err := uuid.Parse(r.Header.Get("X-Idempotency-Key"))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid request header X-Idempotency-Key: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}

	// Body

	var req request
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Errorf("invalid request body: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}
	if dec.More() {
		http.Error(w, "invalid request body: multiple JSONs", http.StatusUnprocessableEntity)
		return
	}

	if req.InputFiles == nil {
		http.Error(w, "invalid request body: missing input_files", http.StatusUnprocessableEntity)
		return
	}
	if len(*req.InputFiles) == 0 {
		http.Error(w, "invalid request body: empty input_files", http.StatusUnprocessableEntity)
		return
	}
	if req.CacheKey == nil {
		http.Error(w, "invalid request body: missing cache_key", http.StatusUnprocessableEntity)
		return
	}
	for k, v := range *req.InputFiles {
		if k == "" {
			http.Error(w, "invalid request body: invalid input_files: a pair has empty key (file path)", http.StatusUnprocessableEntity)
			return
		}
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			http.Error(w, fmt.Errorf("invalid request body: invalid input_files: a pair has invalid value (file content): %w", err).Error(), http.StatusUnprocessableEntity)
			return
		}
	}

	resp := response{BuildID: uuid.New()}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
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
