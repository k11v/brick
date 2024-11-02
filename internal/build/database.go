package build

import (
	"context"

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
func (d *PostgresDatabase) BeginFunc(f func(tx Database) error) error {
	panic("unimplemented")
}

// CreateBuild implements Database.
func (d *PostgresDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuild implements Database.
func (d *PostgresDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuildByIdempotencyKey implements Database.
func (d *PostgresDatabase) GetBuildByIdempotencyKey(params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error) {
	panic("unimplemented")
}

// GetBuildCount implements Database.
func (d *PostgresDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	panic("unimplemented")
}

// ListBuilds implements Database.
func (d *PostgresDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	panic("unimplemented")
}

// LockUser implements Database.
func (d *PostgresDatabase) LockUser(params *DatabaseLockUserParams) error {
	panic("unimplemented")
}
