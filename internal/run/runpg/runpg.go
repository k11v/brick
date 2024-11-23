package runpg

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ConnectionString string // required
}

func NewPool(ctx context.Context, conf Config) (*pgxpool.Pool, error) {
	pgxConf, err := pgxpool.ParseConfig(conf.ConnectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxConf)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
