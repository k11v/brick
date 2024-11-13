package pg

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/k11v/brick/internal/app/build"
)

type row struct {
	ID                uuid.UUID     `db:"id"`
	IdempotencyKey    uuid.UUID     `db:"idempotency_key"`
	UserID            uuid.UUID     `db:"user_id"`
	CreatedAt         time.Time     `db:"created_at"`
	DocumentToken     string        `db:"document_token"`
	ProcessLogToken   string        `db:"process_log_token"`
	ProcessUsedTime   time.Duration `db:"process_used_time"`
	ProcessUsedMemory int           `db:"process_used_memory"`
	ProcessExitCode   int           `db:"process_exit_code"`
	OutputToken       string        `db:"output_token"`
	NextDocumentToken string        `db:"next_document_token"`
	OutputExpiresAt   time.Time     `db:"output_expires_at"`
	Status            string        `db:"status"`
}

func rowToBuild(collectableRow pgx.CollectableRow) (*build.Build, error) {
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, fmt.Errorf("row to build: %w", err)
	}

	status, known := build.StatusFromString(collectedRow.Status)
	if !known {
		slog.Default().Warn(
			"unknown status encountered while creating build",
			"status", collectedRow.Status,
			"build_id", collectedRow.ID,
		)
	}

	b := &build.Build{
		ID:                collectedRow.ID,
		IdempotencyKey:    collectedRow.IdempotencyKey,
		UserID:            collectedRow.UserID,
		CreatedAt:         collectedRow.CreatedAt,
		DocumentToken:     "",
		DocumentFiles:     map[string][]byte{},
		ProcessLogFile:    []byte{},
		ProcessUsedTime:   collectedRow.ProcessUsedTime,
		ProcessUsedMemory: collectedRow.ProcessUsedMemory,
		ProcessExitCode:   collectedRow.ProcessExitCode,
		OutputFile:        []byte{},
		NextDocumentToken: "",
		OutputExpiresAt:   collectedRow.OutputExpiresAt,
		Status:            status,
	}
	return b, nil
}
