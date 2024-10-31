package build

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Database = (*PostgresDatabase)(nil)

type PostgresDatabase struct {
	pool *pgxpool.Pool // required
}

func NewPostgresDatabase(pool *pgxpool.Pool) *PostgresDatabase {
	return &PostgresDatabase{pool: pool}
}

// BeginFunc implements Database.
func (p *PostgresDatabase) BeginFunc(f func(tx Database) error) error {
	panic("unimplemented")
}

// CreateBuild implements Database.
func (p *PostgresDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuild implements Database.
func (p *PostgresDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	return &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
}

// GetBuildByIdempotencyKey implements Database.
func (p *PostgresDatabase) GetBuildByIdempotencyKey(params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error) {
	panic("unimplemented")
}

// GetBuildCount implements Database.
func (p *PostgresDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	panic("unimplemented")
}

// ListBuilds implements Database.
func (p *PostgresDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	panic("unimplemented")
}

// LockUser implements Database.
func (p *PostgresDatabase) LockUser(params *DatabaseLockUserParams) error {
	panic("unimplemented")
}
