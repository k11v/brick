package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

const (
	headerAuthorization   = "Authorization"
	headerXIdempotencyKey = "X-Idempotency-Key"
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
		InputFiles *map[string][]byte `json:"input_files"` // key is path, value is content (decoded from base64)
		CacheKey   *uuid.UUID         `json:"cache_key"`
		Force      *bool              `json:"force"`
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

	// Header Authorization
	if err := checkHeaderCountIsOne(r.Header, headerAuthorization); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID, err := userIDFromAuthorizationHeader(r.Header.Get(headerAuthorization))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s request header: %w", headerAuthorization, err).Error(), http.StatusUnauthorized)
		return
	}
	_ = userID

	// Header X-Idempotency-Key
	if err := checkHeaderCountIsOne(r.Header, headerXIdempotencyKey); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	idempotencyKey, err := uuid.Parse(r.Header.Get(headerXIdempotencyKey))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s request header: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}
	_ = idempotencyKey

	// Body

	var req request
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Errorf("invalid request body: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}
	if dec.More() {
		http.Error(w, "invalid request body: multiple top-level values", http.StatusUnprocessableEntity)
		return
	}

	if req.Force == nil {
		http.Error(w, "invalid request body: missing force", http.StatusUnprocessableEntity)
		return
	}
	if req.CacheKey == nil {
		http.Error(w, "invalid request body: missing cache_key", http.StatusUnprocessableEntity)
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
	for k := range *req.InputFiles {
		if k == "" {
			http.Error(w, "invalid request body: invalid input_files: a pair has empty key (file path)", http.StatusUnprocessableEntity)
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

type Build struct {
	ID         uuid.UUID `json:"id"`
	Done       bool      `json:"done"`
	OutputFile *[]byte   `json:"output_file,omitempty"`
	Error      *string   `json:"error,omitempty"`
}

func (h *handler) GetBuild(w http.ResponseWriter, r *http.Request) {
	// Path value id
	const pathValueID = "id"
	id, err := uuid.Parse(r.PathValue(pathValueID))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %q request path value: %w", pathValueID, err).Error(), http.StatusUnprocessableEntity)
		return
	}
	_ = id

	// Header Authorization
	if err = checkHeaderCountIsOne(r.Header, headerAuthorization); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID, err := userIDFromAuthorizationHeader(r.Header.Get(headerAuthorization))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s request header: %w", headerAuthorization, err).Error(), http.StatusUnauthorized)
		return
	}
	_ = userID

	resp := Build{ID: id, Done: false, OutputFile: nil, Error: nil}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *handler) ListBuilds(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Builds []*Build
		NextPageToken string
		TotalSize int
	}

	// invalidate request with unknown query values?
	// invalidate request with unexpected multiple query values with the same key?

	queryValues, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, fmt.Errorf("invalid request query string: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}

	// Query value filter.
	const queryValueFilter = "filter"
	filter := queryValues.Get(queryValueFilter)
	_ = filter

	// Query value page_token.
	// If the page_token is "", the server will return the first page.
	const queryValuePageToken = "page_token"
	pageToken := queryValues.Get(queryValuePageToken)
	_ = pageToken

	// Query value page_size.
	// If the page_size is 0, the server will decide the number of results to be returned.
	const queryValuePageSize = "page_size"
	pageSize := 0
	if queryValues.Has(queryValuePageSize) {
		pageSize, err = strconv.Atoi(queryValues.Get(queryValuePageSize))
		if err != nil {
			http.Error(w, fmt.Errorf("invalid %q request query value: %w", queryValuePageSize, err).Error(), http.StatusUnprocessableEntity)
			return
		}
	}
	_ = pageSize

	// Header Authorization
	if err = checkHeaderCountIsOne(r.Header, headerAuthorization); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID, err := userIDFromAuthorizationHeader(r.Header.Get(headerAuthorization))
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s request header: %w", headerAuthorization, err).Error(), http.StatusUnauthorized)
		return
	}
	_ = userID

	resp := response{
		Builds: []*Build{
			&Build{ID: uuid.New(), Done: false, OutputFile: nil, Error: nil},
			&Build{ID: uuid.New(), Done: false, OutputFile: nil, Error: nil},
			&Build{ID: uuid.New(), Done: false, OutputFile: nil, Error: nil},
		},
		NextPageToken: "3",
		TotalSize: 7,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
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

func checkHeaderCountIsOne(header http.Header, key string) error {
	if got, want := len(header.Values(key)), 1; got != want {
		if got == 0 {
			return fmt.Errorf("missing %s request header", key)
		} else {
			return fmt.Errorf("multiple %s request headers", key)
		}
	}
	return nil
}

// userIDFromAuthorizationHeader.
// It doesn't check for missing header or multiple headers.
func userIDFromAuthorizationHeader(h string) (uuid.UUID, error) {
	scheme, params, _ := strings.Cut(h, " ")

	if scheme == "" {
		return uuid.UUID{}, errors.New("no scheme")
	}

	if got, want := scheme, "Bearer"; strings.ToLower(got) != strings.ToLower(want) {
		return uuid.UUID{}, fmt.Errorf("got unsupported scheme %q, want %q", got, want)
	}

	// TODO: Replace mock token handling with real.
	userID, err := uuid.Parse(params)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid token: %w", err)
	}

	return userID, nil
}
