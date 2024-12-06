package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

const (
	HeaderAuthorization   = "Authorization"
	HeaderXIdempotencyKey = "X-Idempotency-Key"
)

type Handler struct{}

// When using `_ = m.Ack(false, false)`, we assume that when an error occurs,
// the channel becomes invalid and the code that calls the handler picks up on
// that and recreates the channel. This causes the message to be redelivered.
// The error should be checked if we need to do something about the fact, that
// message wasn't acknowledged and that it will be delivered again, e.g.
// rollback a database transaction.

func (*Handler) RunBuild(m amqp091.Delivery) {
	type message struct {
		Foo string `json:"foo"`
		Bar int    `json:"bar"`
	}

	if err := m.Headers.Validate(); err != nil {
		err = fmt.Errorf("invalid message header: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// Header Authorization.
	authorizationHeader, ok := m.Headers[HeaderAuthorization]
	if !ok {
		err := fmt.Errorf("missing %s message header", HeaderAuthorization)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	token, err := tokenFromAuthorizationHeader(authorizationHeader)
	if err != nil {
		err = fmt.Errorf("invalid %s message header: %w", HeaderAuthorization, err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	_ = token

	// Header X-Idempotency-Key.
	idempotencyKeyHeader, ok := m.Headers[HeaderXIdempotencyKey]
	if !ok {
		err = fmt.Errorf("missing %s message header", HeaderXIdempotencyKey)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	idempotencyKey, err := keyFromIdempotencyKeyHeader(idempotencyKeyHeader)
	if err != nil {
		err = fmt.Errorf("invalid %s message header: %w", HeaderXIdempotencyKey, err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	_ = idempotencyKey

	var msg message
	dec := json.NewDecoder(bytes.NewReader(m.Body))
	if err = dec.Decode(&msg); err != nil {
		err = fmt.Errorf("invalid message body: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	if dec.More() {
		err = errors.New("multiple top-level values")
		err = fmt.Errorf("invalid message body: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
}

func tokenFromAuthorizationHeader(h interface{}) (uuid.UUID, error) {
	hString, ok := h.(string)
	if !ok {
		return uuid.UUID{}, errors.New("not a string")
	}

	scheme, params, _ := strings.Cut(hString, " ")
	if scheme == "" {
		return uuid.UUID{}, errors.New("empty scheme")
	}

	if strings.ToLower(scheme) != "bearer" {
		return uuid.UUID{}, fmt.Errorf("unsupported scheme %s", scheme)
	}

	token, err := uuid.Parse(params)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid params: %w", err)
	}

	return token, nil
}

func keyFromIdempotencyKeyHeader(h interface{}) (uuid.UUID, error) {
	hString, ok := h.(string)
	if !ok {
		return uuid.UUID{}, errors.New("not a string")
	}

	key, err := uuid.Parse(hString)
	if err != nil {
		return uuid.UUID{}, err
	}

	return key, nil
}
