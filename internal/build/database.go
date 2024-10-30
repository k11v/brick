package build

import "github.com/jackc/pgx/v5/pgxpool"

type PostgresDatabase struct {
	pool *pgxpool.Pool
}

var _ Database = (*PostgresDatabase)(nil)

// BeginFunc implements Database.
func (p *PostgresDatabase) BeginFunc(f func(tx Database) error) error {
	panic("unimplemented")
}

// CreateBuild implements Database.
func (p *PostgresDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	panic("unimplemented")
}

// GetBuild implements Database.
func (p *PostgresDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	panic("unimplemented")
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
