package main

import (
	"github.com/caarlos0/env/v11"
)

// config holds the application configuration.
type config struct {
	Development bool `env:"BRICK_DEVELOPMENT"`
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
