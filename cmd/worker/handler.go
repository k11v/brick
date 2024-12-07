package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
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
		ID          *uuid.UUID `json:"id"`
		InputPrefix *string    `json:"input_prefix"`
	}

	if err := m.Headers.Validate(); err != nil {
		err = fmt.Errorf("invalid header: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// Header Authorization.
	authorizationHeader, ok := m.Headers[HeaderAuthorization]
	if !ok {
		err := fmt.Errorf("missing %s header", HeaderAuthorization)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	token, err := tokenFromAuthorizationHeader(authorizationHeader)
	if err != nil {
		err = fmt.Errorf("invalid %s header: %w", HeaderAuthorization, err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	_ = token

	// Header X-Idempotency-Key.
	idempotencyKeyHeader, ok := m.Headers[HeaderXIdempotencyKey]
	if !ok {
		err = fmt.Errorf("missing %s header", HeaderXIdempotencyKey)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	idempotencyKey, err := keyFromIdempotencyKeyHeader(idempotencyKeyHeader)
	if err != nil {
		err = fmt.Errorf("invalid %s header: %w", HeaderXIdempotencyKey, err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	_ = idempotencyKey

	var msg message
	dec := json.NewDecoder(bytes.NewReader(m.Body))
	if err = dec.Decode(&msg); err != nil {
		err = fmt.Errorf("invalid body: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	if dec.More() {
		err = errors.New("multiple top-level values")
		err = fmt.Errorf("invalid body: %w", err)
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// Body field id.
	if msg.ID == nil {
		err = fmt.Errorf("missing %s body field", "id")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// Body field input_prefix.
	if msg.InputPrefix == nil {
		err = fmt.Errorf("missing %s body field", "input_prefix")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}

	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}
	defer func(tempDir string) {
		if err = os.RemoveAll(tempDir); err != nil {
			slog.Default().Error("failed to remove temp dir", "err", err, "temp_dir", tempDir)
		}
	}(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	if err = os.MkdirAll(inputDir, 0o777); err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	// TODO: get InputDirPrefix from Postgres, download from S3
	err = os.WriteFile(
		filepath.Join(inputDir, "main.md"),
		[]byte(`# The Hobbit, or There and Back Again

## Text

Once upon a time, in the depths of the quiet ocean, there lived a small fish named Flora. Flora was special - she had bright colors and long fins that allowed her to swim quickly. She was curious and always eager to explore new places in the ocean.

One day, during her adventures, Flora noticed a large school of her fellow fish migrating north. She decided to join them and explore new places. During the journey, Flora met many different species of fish. She learned that many fish cooperate with each other to find food and protect themselves from predators.

Soon, Flora discovered a huge coral reef community where hundreds of colorful fish lived. They lived in harmony and cared for each other. Flora stayed there and learned a lot from her new friends. She realized that unity and cooperation were key to survival in the ocean.

Over the years, Flora grew older and wiser. She became one of the elders of the coral reef and helped young fish in their journey through the ocean. Her story became a legend among the fish and an inspiration to many. In her old age, Flora felt proud of all she had achieved and thanked the ocean for the amazing adventures and friendships she found along the way.

### Formatting

Text with *italics*.

Text with **bold**.

Text with ***bold italics***.

Text with `+"`code`"+`.

Text with $E = mc^2$.

Text with "'quotes' inside quotes".

Text with 🤔.

Text with кириллица.
`),
		0o666,
	)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	outputDir := filepath.Join(tempDir, "output")
	if err = os.MkdirAll(inputDir, 0o777); err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	result, err := build.Run(&build.RunParams{InputDir: inputDir, OutputDir: outputDir})
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	_ = result.PDFFile  // TODO: get PDFFileKey from Postgres, upload to S3
	_ = result.LogFile  // TODO: get LogFileKey from Postgres, upload to S3
	_ = result.ExitCode // TODO: set ExitCode in Postgres

	// First, I wanted InputDirPrefix to be chosen by the server
	// and PDFFileKey and LogFileKey to be chosen by the worker.
	// But I also wanted these objects to be under the same prefix.
	// Syncing server and worker on this seems complicated,
	// so I decided to make someone responsible for all
	// of these keys and prefixes.
	// Server seems like the logical choice.
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
