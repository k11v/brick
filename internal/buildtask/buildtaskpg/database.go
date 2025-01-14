package buildtaskpg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/k11v/brick/internal/buildtask"
)

var _ buildtask.Database = (*Database)(nil)

type Database struct {
	db Querier // required
}

func NewDatabase(db Querier) *Database {
	return &Database{db: db}
}

// Begin implements buildtask.Database.
func (d *Database) Begin(ctx context.Context) (buildtask.DatabaseTx, error) {
	pgxTx, err := d.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	return newDatabaseTx(pgxTx), nil
}

// LockBuilds implements buildtask.Database.
//
// INSERT with ON CONFLICT DO UPDATE should acquire the FOR UPDATE row-level lock
// when the user_id row exists and acquire a lock when it doesn't exist.
//
// TODO: check the INSERT with ON CONFLICT DO UPDATE command.
func (d *Database) LockBuilds(ctx context.Context, params *buildtask.DatabaseLockBuildsParams) error {
	query := `
		INSERT INTO user_locks (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = excluded.user_id
		RETURNING user_id
	`
	args := []any{params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	_, err := pgx.CollectExactlyOneRow(rows, rowToUUID)
	if err != nil {
		return fmt.Errorf("lock builds: %w", err)
	}

	return nil
}

// CreateBuild implements buildtask.Database.
func (d *Database) CreateBuild(ctx context.Context, params *buildtask.DatabaseCreateBuildParams) (*buildtask.Build, error) {
	query := `
		INSERT INTO builds (idempotency_key, user_id, document_token, status, done)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status, done
	`
	args := []any{params.IdempotencyKey, params.UserID, params.DocumentToken, buildtask.StatusPending, false}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
		return nil, buildtask.ErrIdempotencyKeyAlreadyUsed
	} else if err != nil {
		return nil, fmt.Errorf("create build: %w", err)
	}

	return b, nil
}

// GetBuild implements buildtask.Database.
//
// TODO: Consider silent unmarshalling errors of pgx.CollectExactlyOneRow(rows, rowToBuild)
// here and in other Database methods.
func (d *Database) GetBuild(ctx context.Context, params *buildtask.DatabaseGetBuildParams) (*buildtask.Build, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status, done
		FROM builds
		WHERE id = $1 AND user_id = $2
	`
	args := []any{params.ID, params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, buildtask.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get build: %w", err)
	}

	return b, nil
}

// GetBuildByIdempotencyKey implements buildtask.Database.
func (d *Database) GetBuildByIdempotencyKey(ctx context.Context, params *buildtask.DatabaseGetBuildByIdempotencyKeyParams) (*buildtask.Build, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status, done
		FROM builds
		WHERE idempotency_key = $1 AND user_id = $2
	`
	args := []any{params.IdempotencyKey, params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, buildtask.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get build by idempotency key: %w", err)
	}

	return b, nil
}

// GetBuildCount implements buildtask.Database.
//
// TODO: params.StartTime and params.EndTime could be invalid.
// What should we do about this? Options:
// a) leave it as it is, then the select query will always return zero;
// b) return a validation error;
// c) panic (we consider calling GetBuildCount like that a programming error).
func (d *Database) GetBuildCount(ctx context.Context, params *buildtask.DatabaseGetBuildCountParams) (int, error) {
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

// ListBuilds implements buildtask.Database.
//
// TODO: params.PageLimit and params.PageOffset could be invalid.
// What should we do about that? Options:
// a) leave it as it is, then the select query will always return zero;
// b) return a validation error;
// c) panic.
func (d *Database) ListBuilds(ctx context.Context, params *buildtask.DatabaseListBuildsParams) (*buildtask.DatabaseListBuildsResult, error) {
	query := `
		SELECT
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status, done
		FROM builds
		WHERE user_id = $1
		ORDER BY created_at DESC, id ASC
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

	result := &buildtask.DatabaseListBuildsResult{
		Builds:         builds,
		NextPageOffset: nextPageOffset,
		TotalSize:      totalSize,
	}
	return result, nil
}

func (d *Database) UpdateBuild(ctx context.Context, params *buildtask.DatabaseUpdateBuildParams) (*buildtask.Build, error) {
	query := `
		UPDATE builds
		SET ...
		WHERE id = $3 AND user_id = $4
		RETURNING
			id, idempotency_key,
			user_id, created_at,
			document_token,
			process_log_token, process_used_time, process_used_memory, process_exit_code,
			output_token, next_document_token, output_expires_at,
			status, done
	`
	args := []any{params.ExitCode, params.Status, params.ID, params.UserID}

	rows, _ := d.db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, buildtask.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("update build: %w", err)
	}

	return b, nil
}
