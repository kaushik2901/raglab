package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}

	if err := retroactivelyRecord001(ctx, pool); err != nil {
		return fmt.Errorf("retroactively record 001: %w", err)
	}

	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)

	for _, entry := range entries {
		version := migrationVersion(entry)
		if version == "" {
			continue
		}

		applied, err := isMigrationApplied(ctx, pool, version)
		if err != nil {
			return fmt.Errorf("check %s: %w", entry, err)
		}
		if applied {
			continue
		}

		sql, err := migrationsFS.ReadFile(entry)
		if err != nil {
			return fmt.Errorf("read %s: %w", entry, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", entry, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("execute %s: %w", entry, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			version,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", entry, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", entry, err)
		}
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}

	return nil
}

func ensureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func retroactivelyRecord001(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'workflows')`,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ('001') ON CONFLICT (version) DO NOTHING`,
	)
	return err
}

func isMigrationApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var applied bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
		version,
	).Scan(&applied)
	return applied, err
}

func migrationVersion(filePath string) string {
	name := path.Base(filePath)
	name = strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}
