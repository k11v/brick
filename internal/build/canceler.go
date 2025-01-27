package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAlreadyDoing = errors.New("already doing")
	ErrAlreadyDone  = errors.New("already done")
)

type Status string

const (
	StatusTodo  Status = "todo"
	StatusDoing Status = "doing"
	StatusDone  Status = "done"
)

func ParseStatus(s string) (status Status, known bool) {
	status = Status(s)
	switch status {
	case StatusTodo, StatusDoing, StatusDone:
		return status, true
	default:
		return status, false
	}
}

type Canceler struct {
	DB *pgxpool.Pool
}

func NewCanceler(db *pgxpool.Pool) *Canceler {
	return &Canceler{DB: db}
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

	b, err := getForUpdate(ctx, tx, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}
	if b.UserID != params.UserID {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAccessDenied)
	}

	if b.Status == StatusDoing {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAlreadyDoing)
	}
	if b.Status == StatusDone {
		return nil, fmt.Errorf("build.Canceler: %w", ErrAlreadyDone)
	}

	b, err = updateStatus(ctx, tx, params.ID, StatusDone, ErrorCanceled)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Canceler: %w", err)
	}

	return b, nil
}

func getForUpdate(ctx context.Context, db executor, id uuid.UUID) (*Build, error) {
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

func updateStatus(ctx context.Context, db executor, id uuid.UUID, status Status, errorValue Error) (*Build, error) {
	query := `
		UPDATE builds
		SET status = $2, error = $3
		WHERE id = $1
		RETURNING id, created_at, idempotency_key, user_id, status, error, exit_code, log_data_key, output_data_key
	`
	args := []any{id, string(status), string(errorValue)}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}
