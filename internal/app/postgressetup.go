package app

import (
	"database/sql"
	"embed"
	"errors"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func SetupPostgres(connectionString string) error {
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	return migratePostgresDB(db)
}

//go:embed migrations/*.sql
var postgresMigrations embed.FS

func postgresMigrationsFS() fs.FS {
	sub, err := fs.Sub(postgresMigrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

func migratePostgresDB(db *sql.DB) error {
	sourceDriver, err := iofs.New(postgresMigrationsFS(), ".")
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
