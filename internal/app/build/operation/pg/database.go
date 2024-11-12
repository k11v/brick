package pg

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/k11v/brick/internal/app/build"
	"github.com/k11v/brick/internal/app/build/operation"
)

type Database struct {
	db Querier // required
}

// TODO: Remove.
var _ operation.Database = (*Database)(nil)

func NewDatabase(db Querier) *Database {
	return &Database{db: db}
}

// BeginFunc implements Database.
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

// CreateBuild implements Database.
func (d *Database) CreateBuild(ctx context.Context, params *operation.DatabaseCreateBuildParams) (*build.Build, error) {
	return &build.Build{
		// Done:             false,
		// Error:            nil,
		ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		// NextContextToken: "",
		OutputFile: nil,
	}, nil
}

// GetBuild implements Database.
func (d *Database) GetBuild(ctx context.Context, params *operation.DatabaseGetBuildParams) (*build.Build, error) {
	return &build.Build{
		// Done:             false,
		// Error:            nil,
		ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		// NextContextToken: "",
		OutputFile: nil,
	}, nil
}

// GetBuildByIdempotencyKey implements Database.
func (d *Database) GetBuildByIdempotencyKey(ctx context.Context, params *operation.DatabaseGetBuildByIdempotencyKeyParams) (*build.Build, error) {
	panic("unimplemented")
}

// GetBuildCount implements Database.
func (d *Database) GetBuildCount(ctx context.Context, params *operation.DatabaseGetBuildCountParams) (int, error) {
	panic("unimplemented")
}

// ListBuilds implements Database.
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

// LockUser implements Database.
func (d *Database) LockUser(ctx context.Context, params *operation.DatabaseLockUserParams) error {
	panic("unimplemented")
}
