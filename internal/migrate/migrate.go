// Package migrate wraps golang-migrate so the rest of the app (main.go, and
// this package's own tests) has a two-function surface: Up and Down.
//
// Driver choice: this package uses golang-migrate's standard lib/pq-backed
// "postgres" database driver (via the blank import below) rather than its
// pgx-based driver. The app's own runtime queries (from Task 5 onward) will
// use pgx per the project's tech-stack decision, but the migration runner
// itself has no need to share that driver — the lib/pq-backed driver is
// golang-migrate's most common, best-documented path, which keeps this
// narrowly-scoped package simple.
//
// Migration source: the *.sql files live in migrations/ at the repo root
// (source of truth) and are embedded into the binary via migrations.FS
// (migrations/migrations.go), so the compiled server does not depend on a
// migrations/ directory existing on disk at runtime.
package migrate

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"DragonMarket/migrations"
)

// newMigrate builds a *migrate.Migrate backed by the embedded migrations
// and the given Postgres connection string.
func newMigrate(databaseURL string) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: load embedded source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrate: init: %w", err)
	}
	return m, nil
}

// Up runs all pending migrations. It is a no-op (returns nil) if the schema
// is already at the latest version.
func Up(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}

// Down reverts all migrations. Used by tests to verify the down migrations
// are clean; not called from normal app startup.
func Down(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: down: %w", err)
	}
	return nil
}
