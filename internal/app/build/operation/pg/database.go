package pg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
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

	rows, _ := d.db.Query(ctx, query, args)
	resultRow, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, fmt.Errorf("database create build: %w", err)
	}

	status, known := build.StatusFromString(resultRow.Status)
	if !known {
		slog.Default().Warn(
			"unknown status encountered while creating build",
			"status", resultRow.Status,
			"build_id", resultRow.ID,
		)
	}

	b := &build.Build{
		ID:                resultRow.ID,
		IdempotencyKey:    resultRow.IdempotencyKey,
		UserID:            resultRow.UserID,
		CreatedAt:         resultRow.CreatedAt,
		DocumentToken:     "",
		DocumentFiles:     map[string][]byte{},
		ProcessLogFile:    []byte{},
		ProcessUsedTime:   resultRow.ProcessUsedTime,
		ProcessUsedMemory: resultRow.ProcessUsedMemory,
		ProcessExitCode:   resultRow.ProcessExitCode,
		OutputFile:        []byte{},
		NextDocumentToken: "",
		OutputExpiresAt:   resultRow.OutputExpiresAt,
		Status:            status,
	}
	return b, nil
}

// GetBuild implements operation.Database.
func (d *Database) GetBuild(ctx context.Context, params *operation.DatabaseGetBuildParams) (*build.Build, error) {
	panic("unimplemented")
}

// GetBuildByIdempotencyKey implements operation.Database.
func (d *Database) GetBuildByIdempotencyKey(ctx context.Context, params *operation.DatabaseGetBuildByIdempotencyKeyParams) (*build.Build, error) {
	panic("unimplemented")
}

// GetBuildCount implements operation.Database.
func (d *Database) GetBuildCount(ctx context.Context, params *operation.DatabaseGetBuildCountParams) (int, error) {
	panic("unimplemented")
}

func (d *Database) ListBuilds(ctx context.Context, params *operation.DatabaseListBuildsParams) (*operation.DatabaseListBuildsResult, error) {
	// Currently db struct tags aren't used.
	type row struct {
		ID             uuid.UUID `db:"id"`
		CreatedAt      time.Time `db:"created_at"`
		IdempotencyKey uuid.UUID `db:"idempotency_key"`
		Status         string    `db:"status"`
		UserID         uuid.UUID `db:"user_id"`
	}

	rows, err := d.db.Query(ctx, `SELECT id, created_at, idempotency_key, status, user_id FROM builds`)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var r row
		if err = rows.Scan(&r.ID, &r.CreatedAt, &r.IdempotencyKey, &r.Status, &r.UserID); err != nil {
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	panic("unimplemented")
}

// LockUser implements operation.Database.
func (d *Database) LockUser(ctx context.Context, params *operation.DatabaseLockUserParams) error {
	panic("unimplemented")
}
