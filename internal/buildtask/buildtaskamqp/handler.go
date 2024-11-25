package buildtaskamqp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

const (
	headerAuthorization   = "Authorization"
	headerXIdempotencyKey = "X-Idempotency-Key"
)

type Handler struct{}

func (*Handler) RunBuildTask(ch *amqp091.Channel, m *amqp091.Delivery) {
	type message struct {
		ID             uuid.UUID
		IdempotencyKey uuid.UUID

		UserID    uuid.UUID
		CreatedAt time.Time

		DocumentToken string // instead of DocumentCacheFiles map[string][]byte
		DocumentFiles map[string][]byte

		ProcessLogFile    []byte
		ProcessUsedTime   time.Duration
		ProcessUsedMemory int
		ProcessExitCode   int

		OutputFile        []byte
		NextDocumentToken string // instead of OutputCacheFiles map[string][]byte
		OutputExpiresAt   time.Time

		Status Status
		Done   bool
	}

	// Header Authorization.
	authorizationHeader, ok := m.Headers[headerAuthorization]
	m.Headers.Validate()
	if !ok {
		err := fmt.Errorf("missing %s message header", headerAuthorization)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID, err := userIDFromAuthorizationHeader(authorizationHeader)
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s message header: %w", headerAuthorization, err).Error(), http.StatusUnauthorized)
		return
	}
	_ = userID

	// Header X-Idempotency-Key.
	idempotencyHeader, ok := m.Headers[headerXIdempotencyKey]
	if !ok {
		err := fmt.Errorf("missing %s message header", headerXIdempotencyKey)
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	idempotencyHeaderString, ok := idempotencyHeader.(string)
	if !ok {
		err := fmt.Errorf("invalid %s message header: not a string", headerXIdempotencyKey)
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	idempotencyKey, err := uuid.Parse(idempotencyHeaderString)
	if err != nil {
		http.Error(w, fmt.Errorf("invalid %s message header: %w", headerXIdempotencyKey, err).Error(), http.StatusUnprocessableEntity)
		return
	}
	_ = idempotencyKey

	var msg message
	dec := json.NewDecoder(bytes.NewReader(m.Body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&msg); err != nil {
		http.Error(w, fmt.Errorf("invalid request body: %w", err).Error(), http.StatusUnprocessableEntity)
		return
	}
	if dec.More() {
		http.Error(w, "invalid request body: multiple top-level values", http.StatusUnprocessableEntity)
		return
	}

	// STOPPED HERE

	// Body field done.
	if msg.Done == nil {
		http.Error(w, "invalid request body: missing force", http.StatusUnprocessableEntity)
		return
	}

	// Body field cache_key.
	if msg.CacheKey == nil {
		http.Error(w, "invalid request body: missing cache_key", http.StatusUnprocessableEntity)
		return
	}

	// Body field input_files.
	if msg.InputFiles == nil {
		http.Error(w, "invalid request body: missing input_files", http.StatusUnprocessableEntity)
		return
	}
	if len(*msg.InputFiles) == 0 {
		http.Error(w, "invalid request body: empty input_files", http.StatusUnprocessableEntity)
		return
	}
	for k := range *msg.InputFiles {
		if k == "" {
			http.Error(w, "invalid request body: invalid input_files: a pair has empty key (file path)", http.StatusUnprocessableEntity)
			return
		}
	}

	resp := response{ID: uuid.New(), Done: false, OutputFile: nil, Error: nil}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// FIXME: http.Error wouldn't get a chance to change the headers and status code
		// because of http.ResponseWriter.WriteHeader call earlier.
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// userIDFromAuthorizationHeader.
// It doesn't check for missing header or multiple headers.
func userIDFromAuthorizationHeader(h interface{}) (uuid.UUID, error) {
	headerString, ok := h.(string)
	if !ok {
		return uuid.UUID{}, errors.New("not a string")
	}

	scheme, params, _ := strings.Cut(headerString, " ")

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
