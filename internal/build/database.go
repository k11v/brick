package build

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Database = (*PostgresDatabase)(nil)

var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = pgx.Tx(nil)
)

type Querier interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (commandTag pgconn.CommandTag, err error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type PostgresDatabase struct {
	db Querier // required
}

func NewPostgresDatabase(db Querier) *PostgresDatabase {
	return &PostgresDatabase{db: db}
}

// BeginFunc implements Database.
func (d *PostgresDatabase) BeginFunc(ctx context.Context, f func(tx Database) error) error {
	tx, err := d.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func(tx pgx.Tx) {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			slog.Default().Error("failed to rollback", "error", rollbackErr)
		}
	}(tx)

	txDatabase := NewPostgresDatabase(tx)
	if err = f(txDatabase); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CreateBuild implements Database.
func (d *PostgresDatabase) CreateBuild(ctx context.Context, params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuild implements Database.
func (d *PostgresDatabase) GetBuild(ctx context.Context, params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuildByIdempotencyKey implements Database.
func (d *PostgresDatabase) GetBuildByIdempotencyKey(ctx context.Context, params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error) {
	panic("unimplemented")
}

// GetBuildCount implements Database.
func (d *PostgresDatabase) GetBuildCount(ctx context.Context, params *DatabaseGetBuildCountParams) (int, error) {
	panic("unimplemented")
}

// ListBuilds implements Database.
func (d *PostgresDatabase) ListBuilds(ctx context.Context, params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
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
func (d *PostgresDatabase) LockUser(ctx context.Context, params *DatabaseLockUserParams) error {
	panic("unimplemented")
}
