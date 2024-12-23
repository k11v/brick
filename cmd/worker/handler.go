package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
)

const (
	HeaderAuthorization   = "Authorization"
	HeaderXIdempotencyKey = "X-Idempotency-Key"
)

type Handler struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
}

func (h *Handler) Run(m amqp091.Delivery) {
	ctx := context.Background()

	type message struct {
		ID *uuid.UUID `json:"id"`
	}

	err := m.Headers.Validate()
	if err != nil {
		err = fmt.Errorf("invalid header: %w", err)
		slog.Error("", "err", err)
		_ = m.Nack(false, false)
		return
	}

	var msg message
	dec := json.NewDecoder(bytes.NewReader(m.Body))
	err = dec.Decode(&msg)
	if err != nil {
		err = fmt.Errorf("invalid body: %w", err)
		slog.Error("", "err", err)
		_ = m.Nack(false, false)
		return
	}
	if dec.More() {
		err = errors.New("multiple top-level values")
		err = fmt.Errorf("invalid body: %w", err)
		slog.Error("", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// Body field id.
	if msg.ID == nil {
		err = fmt.Errorf("missing %s body field", "id")
		slog.Error("", "err", err)
		_ = m.Nack(false, false)
		return
	}
	id := *msg.ID

	operationRunner := &build.BuildRunner{DB: h.DB, S3: h.S3}
	_, err = operationRunner.Run(ctx, &build.BuildRunnerRunParams{ID: id})
	if err != nil {
		slog.Error("", "err", err)
		_ = m.Nack(false, false)
		return
	}

	_ = m.Ack(false)
}
