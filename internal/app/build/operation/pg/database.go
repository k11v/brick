package pg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/k11v/brick/internal/app/build"
	"github.com/k11v/brick/internal/app/build/operation"
)

var _ operation.Database = (*Database)(nil)

type Database struct {
	db Querier // required
}

func NewDatabase(db Querier) *Database {
	return &Database{db: db}
}

// BeginFunc implements operation.Database.
func (d *Database) BeginFunc(ctx context.Context, f func(tx operation.Database) error) error {
	tx, err := d.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func(tx pgx.Tx) {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			slog.Default().Error("failed to rollback", "error", rollbackErr)
		}
	}(tx)

	txDatabase := NewDatabase(tx)
	if err = f(txDatabase); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CreateBuild implements operation.Database.
func (d *Database) CreateBuild(ctx context.Context, params *operation.DatabaseCreateBuildParams) (*build.Build, error) {
	query := `
		INSERT INTO builds (idempotency_key, user_id, document_token, status)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status
	`
	args := []any{params.IdempotencyKey, params.UserID, params.DocumentToken, "pending"}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, fmt.Errorf("create build: %w", err)
	}

	return b, nil
}

// GetBuild implements operation.Database.
func (d *Database) GetBuild(ctx context.Context, params *operation.DatabaseGetBuildParams) (*build.Build, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status
		FROM builds
		WHERE id = $1 AND user_id = $2
	`
	args := []any{params.ID, params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, fmt.Errorf("get build: %w", err)
	}

	return b, nil
}

// GetBuildByIdempotencyKey implements operation.Database.
func (d *Database) GetBuildByIdempotencyKey(ctx context.Context, params *operation.DatabaseGetBuildByIdempotencyKeyParams) (*build.Build, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status
		FROM builds
		WHERE idempotency_key = $1 AND user_id = $2
	`
	args := []any{params.IdempotencyKey, params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, fmt.Errorf("get build by idempotency key: %w", err)
	}

	return b, nil
}

// GetBuildCount implements operation.Database.
func (d *Database) GetBuildCount(ctx context.Context, params *operation.DatabaseGetBuildCountParams) (int, error) {
	query := `
		SELECT count(*)
		FROM builds
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`
	args := []any{params.UserID, params.StartTime, params.EndTime}

	rows, _ := d.db.Query(ctx, query, args...)
	count, err := pgx.CollectExactlyOneRow(rows, rowToInt)
	if err != nil {
		return 0, fmt.Errorf("get build count: %w", err)
	}

	return count, nil
}

func (d *Database) ListBuilds(ctx context.Context, params *operation.DatabaseListBuildsParams) (*operation.DatabaseListBuildsResult, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status
		FROM builds
		WHERE user_id = $1
		LIMIT $2
		OFFSET $3
	`
	args := []any{params.UserID, params.PageLimit, params.PageOffset}

	rows, _ := d.db.Query(ctx, query, args...)
	builds, err := pgx.CollectRows(rows, rowToBuild)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}

	query = `
		SELECT count(*)
		FROM builds
		WHERE user_id = $1
	`
	args = []any{params.UserID}

	rows, _ = d.db.Query(ctx, query, args...)
	totalSize, err := pgx.CollectExactlyOneRow(rows, rowToInt)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}

	nextPageOffset := new(int)
	*nextPageOffset = params.PageOffset + len(builds)
	if *nextPageOffset >= totalSize {
		nextPageOffset = nil
	}

	result := &operation.DatabaseListBuildsResult{
		Builds:         builds,
		NextPageOffset: nextPageOffset,
		TotalSize:      totalSize,
	}
	return result, nil
}

// LockUser implements operation.Database.
func (d *Database) LockUser(ctx context.Context, params *operation.DatabaseLockUserParams) error {
	panic("unimplemented")
}