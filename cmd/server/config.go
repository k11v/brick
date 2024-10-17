package main

import (
	"github.com/caarlos0/env/v11"
	"github.com/k11v/brick/internal/postgresutil"
	"github.com/k11v/brick/internal/server"
)

// config holds the application configuration.
type config struct {
	Development bool                `env:"BRICK_DEVELOPMENT"`
	Postgres    postgresutil.Config `envPrefix:"BRICK_POSTGRES_"`
	Server      server.Config       `envPrefix:"BRICK_SERVER_"`
}

// parseConfig parses the application configuration from the environment variables.
func parseConfig(environ []string) (*config, error) {
	var cfg config

	err := env.ParseWithOptions(&cfg, env.Options{
		Environment: env.ToMap(environ),
	})
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
