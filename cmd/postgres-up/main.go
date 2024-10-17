package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/k11v/brick/internal/postgresutil"
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
	log := slog.Default()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := newDB(ctx, &cfg.Postgres)
	if err != nil {
		return err
	}
	defer closeWithLog(db, log)

	if err = migrateDB(db); err != nil {
		return err
	}

	return nil
}

func newDB(ctx context.Context, cfg *postgresutil.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

func migrateDB(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS(), ".")
	if err != nil {
		return fmt.Errorf("failed to create migrate source driver: %w", err)
	}

	databaseDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrate database driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrate: %w", err)
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to migrate: %w", err)
	}
	return nil
}

func closeWithLog(c io.Closer, log *slog.Logger) {
	if err := c.Close(); err != nil {
		log.Error("failed to close", "error", err)
	}
}
