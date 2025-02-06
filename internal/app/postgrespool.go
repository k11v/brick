package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	pgxConf, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxConf)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
