// Command migrate applies goose SQL migrations from migrations/ against
// DATABASE_URL.
//
// This wraps goose's library, not its CLI (cmd/goose). The CLI statically
// links a driver for every database goose supports, so `go get`-ing it pulls
// in ClickHouse, MSSQL, YDB and SQLite as dependencies of a Postgres-only
// project. Importing the core package and registering only the Postgres
// driver this project already depends on avoids that.
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Kontora20/where-to-bird-today/internal/config"
)

const migrationsDir = "migrations"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: migrate <up|down|status>")
	}
	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// goose drives migrations through database/sql, not pgx's native pool.
	// The blank import above registers pgx as a database/sql driver under the
	// name "pgx", so this still goes over the same driver as the rest of the
	// app rather than pulling in database/sql's default (lib/pq-style) path.
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	// A close error here has nothing left to report to: the program is
	// about to exit either way, and the migration result above already
	// determines success or failure.
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting dialect: %w", err)
	}

	switch command {
	case "up":
		err = goose.Up(db, migrationsDir)
	case "down":
		err = goose.Down(db, migrationsDir)
	case "status":
		err = goose.Status(db, migrationsDir)
	default:
		return fmt.Errorf("unknown command %q: use up, down, or status", command)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}
	return nil
}
