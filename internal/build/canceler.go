package build

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "done.succeeded"
	StatusFailed    Status = "done.failed"
	StatusCanceled  Status = "done.canceled"
)

var (
	ErrAlreadyDone    = errors.New("already done")
	ErrAlreadyRunning = errors.New("already running")
)

type Canceler struct {
	DB *pgxpool.Pool       // required
	MQ *amqp091.Connection // required
	S3 *s3.Client          // required
}

type CancelerCancelParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (c *Canceler) Cancel(ctx context.Context, params *CancelerCancelParams) (*Build, error) {
	tx, err := c.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	b, err := getBuildForUpdate(ctx, tx, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}
	if b.UserID != params.UserID {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAccessDenied)
	}

	if strings.Split(string(b.Status), ".")[0] == "running" {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAlreadyRunning)
	}
	if strings.Split(string(b.Status), ".")[0] == "done" {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAlreadyDone)
	}

	b, err = updateBuildStatus(ctx, tx, params.ID, StatusCanceled)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}

	return b, nil
}

func getBuildForUpdate(ctx context.Context, db executor, id uuid.UUID) (*Build, error) {
	query := `
		SELECT id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
		FROM builds
		WHERE id = $1
		FOR UPDATE
	`
	args := []any{id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return b, nil
}

func updateBuildStatus(ctx context.Context, db executor, id uuid.UUID, status Status) (*Build, error) {
	query := `
		UPDATE builds
		SET status = $1
		WHERE id = $2
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
	`
	args := []any{string(status), id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}
