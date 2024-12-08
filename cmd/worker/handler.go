package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
	"github.com/k11v/brick/internal/buildtask"
	"github.com/k11v/brick/internal/buildtask/buildtaskpg"
	"github.com/k11v/brick/internal/buildtask/buildtasks3"
)

const (
	HeaderAuthorization   = "Authorization"
	HeaderXIdempotencyKey = "X-Idempotency-Key"
)

type Handler struct {
	database *buildtaskpg.Database
	storage  *buildtasks3.Storage
}

// When using `_ = m.Ack(false, false)`, we assume that when an error occurs,
// the channel becomes invalid and the code that calls the handler picks up on
// that and recreates the channel. This causes the message to be redelivered.
// The error should be checked if we need to do something about the fact, that
// message wasn't acknowledged and that it will be delivered again, e.g.
// rollback a database transaction.

func (h *Handler) RunBuild(m amqp091.Delivery) {
	ctx := context.Background()
	_ = ctx

	type message struct {
		ID               *uuid.UUID `json:"id"`
		UserID           *uuid.UUID `json:"user_id"`
		InputDirPrefix   *string    `json:"input_dir_prefix"`
		OutputPDFFileKey *string    `json:"output_pdf_file_key"`
		OutputLogFileKey *string    `json:"output_log_file_key"`
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
	id := *msg.ID

	// Body field user_id.
	if msg.UserID == nil {
		err = fmt.Errorf("missing %s body field", "user_id")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	userID := *msg.UserID

	// Body field input_dir_prefix.
	if msg.InputDirPrefix == nil {
		err = fmt.Errorf("missing %s body field", "input_dir_prefix")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	inputDirPrefix := *msg.InputDirPrefix

	// Body field output_pdf_file_key.
	if msg.OutputPDFFileKey == nil {
		err = fmt.Errorf("missing %s body field", "output_pdf_file_key")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	outputPDFFileKey := *msg.OutputPDFFileKey

	// Body field output_log_file_key.
	if msg.OutputLogFileKey == nil {
		err = fmt.Errorf("missing %s body field", "output_log_file_key")
		slog.Default().Error("got invalid message", "err", err)
		_ = m.Nack(false, false)
		return
	}
	outputLogFileKey := *msg.OutputLogFileKey

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

	mr, err := h.storage.DownloadDirV2(ctx, inputDirPrefix)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	for {
		fileNamePart, err := mr.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			slog.Default().Error("failed to run", "err", err)
			_ = m.Nack(false, false)
			return
		}
		fileNameBytes, err := io.ReadAll(fileNamePart)
		if err != nil {
			slog.Default().Error("failed to run", "err", err)
			_ = m.Nack(false, false)
			return
		}
		fileName := string(fileNameBytes)

		contentPart, err := mr.NextPart()
		if err != nil {
			slog.Default().Error("failed to run", "err", err)
			_ = m.Nack(false, false)
			return
		}

		file := filepath.Join(inputDir, fileName)
		openFile, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			slog.Default().Error("failed to run", "err", err)
			_ = m.Nack(false, false)
			return
		}
		_, err = io.Copy(openFile, contentPart)
		if err != nil {
			slog.Default().Error("failed to run", "err", err)
			_ = m.Nack(false, false)
			return
		}
		_ = openFile.Close() // TODO: defer
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

	openPDFFile, err := os.Open(result.PDFFile)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}
	err = h.storage.UploadFileV2(ctx, outputPDFFileKey, openPDFFile)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	openLogFile, err := os.Open(result.LogFile)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}
	err = h.storage.UploadFileV2(ctx, outputLogFileKey, openLogFile)
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	_, err = h.database.UpdateBuild(ctx, &buildtask.DatabaseUpdateBuildParams{
		ID:       id,
		UserID:   userID,
		ExitCode: newInt(result.ExitCode),
		Status:   newStatus(buildtask.StatusCompleted),
	})
	if err != nil {
		slog.Default().Error("failed to run", "err", err)
		_ = m.Nack(false, false)
		return
	}

	_ = m.Ack(false)
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

func newInt(v int) *int {
	return &v
}

func newStatus(v buildtask.Status) *buildtask.Status {
	return &v
}
