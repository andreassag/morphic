package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	content string
}

// RunMigrations executes pending SQL migrations in version order.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure migrations tracking table exists
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INT PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	// Read applied versions
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version ASC")
	if err != nil {
		return fmt.Errorf("querying applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scanning migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading migration rows: %w", err)
	}

	// Discover embedded migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		base := entry.Name()
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		content, err := migrationsFS.ReadFile(filepath.Join("migrations", base))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", base, err)
		}
		migrations = append(migrations, migration{
			version: v,
			name:    base,
			content: string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Execute unapplied migrations in transaction
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		slog.Info("applying database migration", "version", m.version, "name", m.name)

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("starting transaction for migration %s: %w", m.name, err)
		}

		if _, err := tx.Exec(ctx, m.content); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("executing migration %s: %w", m.name, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.version, m.name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("recording migration %s: %w", m.name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing migration %s: %w", m.name, err)
		}

		slog.Info("applied migration successfully", "version", m.version, "name", m.name)
	}

	return nil
}
