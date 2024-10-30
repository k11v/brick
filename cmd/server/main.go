package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	_ "github.com/k11v/brick/cmd/server/docs"
	"github.com/k11v/brick/internal/postgresutil"
	"github.com/k11v/brick/internal/server"
)

//go:generate go run github.com/swaggo/swag/cmd/swag init -g main.go -o docs

//	@title			Brick API
//	@version		0.0
//	@description	Brick is a service that builds PDF files from Markdown files.
//	@termsOfService	http://brick.k11v.cc/terms

//	@contact.name	Brick Support
//	@contact.url	http://brick.k11v.cc/support

//	@license.name	MIT License
//	@license.url	https://opensource.org/licenses/MIT

//	@externalDocs.description	Brick Docs
//	@externalDocs.url			https://brick.k11v.cc/docs

//	@host		brick.k11v.cc
//	@BasePath	/api/v1
//	@schemes	https

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

	postgresPool, err := postgresutil.NewPool(ctx, cfg.Postgres.DSN)
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
