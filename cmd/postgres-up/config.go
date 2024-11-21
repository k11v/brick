package main

import (
	"github.com/caarlos0/env/v11"

	"github.com/k11v/brick/internal/pgutil"
)

// config holds the application configuration.
type config struct {
	Postgres pgutil.Config `envPrefix:"BRICK_POSTGRES_"`
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
