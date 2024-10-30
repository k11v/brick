package postgresutil

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
