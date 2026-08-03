package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/yourusername/traccar-billing/migrations"
)

func RunMigrations(db *sql.DB, driver string) error {
	var (
		dbDriver database.Driver
		srcFS    fs.FS
		dir      string
		err      error
	)

	switch driver {
	case "mysql":
		dbDriver, err = migratemysql.WithInstance(db, &migratemysql.Config{})
		srcFS, dir = migrations.MySQL, "mysql"
	case "sqlite":
		dbDriver, err = migratesqlite.WithInstance(db, &migratesqlite.Config{})
		srcFS, dir = migrations.SQLite, "sqlite"
	default:
		return fmt.Errorf("storage: unsupported driver %q", driver)
	}
	if err != nil {
		return fmt.Errorf("storage: create migration driver: %w", err)
	}

	source, err := iofs.New(srcFS, dir)
	if err != nil {
		return fmt.Errorf("storage: create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, driver, dbDriver)
	if err != nil {
		return fmt.Errorf("storage: create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("storage: run migrations: %w", err)
	}
	return nil
}
