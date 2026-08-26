package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies every pending schema migration from the embedded
// migrations/ directory. Migrations are compiled into the binary (rather
// than read from disk at deploy time) so the Docker image and any other
// deployment target only ever needs the single built executable.
//
// It opens a short-lived database/sql connection purely to drive the
// migration tool, since golang-migrate's postgres driver expects *sql.DB;
// all application queries elsewhere continue to use the pgxpool.Pool.
func RunMigrations(connString string) error {
	if connString == "" {
		return fmt.Errorf("run migrations: DATABASE_URL is empty")
	}

	sqlDB, err := sql.Open("pgx", connString)
	if err != nil {
		return fmt.Errorf("run migrations: open connection: %w", err)
	}
	defer func() {
		if cerr := sqlDB.Close(); cerr != nil {
			fmt.Printf("run migrations: warning: close migration connection: %v\n", cerr)
		}
	}()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("run migrations: init postgres driver: %w", err)
	}

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("run migrations: init source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("run migrations: init migrator: %w", err)
	}

	// Auto-heal dirty migrations if a previous deployment failed midway
	v, dirty, vErr := m.Version()
	if vErr == nil && dirty {
		forceTarget := int(v) - 1
		if forceTarget < 0 {
			forceTarget = 0
		}
		if fErr := m.Force(forceTarget); fErr != nil {
			return fmt.Errorf("run migrations: auto-healing dirty version %d: %w", v, fErr)
		}
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: apply: %w", err)
	}

	return nil
}
