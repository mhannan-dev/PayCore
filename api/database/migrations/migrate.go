package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var sqlFiles embed.FS

type migration struct {
	version int
	name    string
}

func orderedMigrations() ([]migration, error) {
	names, err := fs.Glob(sqlFiles, "*.sql")
	if err != nil {
		return nil, err
	}
	var result []migration
	seen := make(map[int]bool)
	for _, name := range names {
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version <= 0 || seen[version] {
			return nil, fmt.Errorf("invalid or duplicate migration version: %s", name)
		}
		seen[version] = true
		result = append(result, migration{version: version, name: name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	return result, nil
}

// Apply serializes versioned migrations across simultaneous API startups.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	steps, err := orderedMigrations()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(724019331)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())"); err != nil {
		return err
	}
	for _, step := range steps {
		var applied bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", step.version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		contents, err := sqlFiles.ReadFile(step.name)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("migration %s: %w", step.name, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES ($1)", step.version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
