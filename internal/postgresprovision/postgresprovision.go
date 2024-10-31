package postgresprovision

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Setup(connectionString string) error {
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return err
	}
	defer closeWithLog(db)

	return migrateDB(db)
}

func migrateDB(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS(), ".")
	if err != nil {
		return err
	}

	databaseDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return err
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

func closeWithLog(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Default().Error("failed to close", "error", err)
	}
}
