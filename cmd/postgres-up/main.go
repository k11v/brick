package main

import (
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/k11v/brick/internal/postgresprovision"
)

func main() {
	if err := run(os.Environ()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run(environ []string) error {
	cfg, err := parseConfig(environ)
	if err != nil {
		return err
	}

	return postgresprovision.Setup(cfg.Postgres.DSN)
}
