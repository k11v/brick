package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/k11v/brick/internal/postgresutil"
	"github.com/k11v/brick/internal/server"
)

func main() {
	if err := run(os.Stdout, os.Environ()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run(stdout io.Writer, environ []string) error {
	cfg, err := parseConfig(environ)
	if err != nil {
		return err
	}
	log := newLogger(stdout, cfg.Development)

	ctx := context.Background()

	postgresPool, err := postgresutil.NewPool(ctx, log, &cfg.Postgres, cfg.Development)
	if err != nil {
		return err
	}
	defer postgresPool.Close()

	srv := server.New(&cfg.Server, log, postgresPool)

	log.Info(
		"starting server",
		"addr", srv.Addr,
		"development", cfg.Development,
	)
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
