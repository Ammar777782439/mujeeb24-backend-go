package database

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/migrations"
	"github.com/jackc/pgx/v5"
)

const migrationTable = "schema_migrations"

var migrationFilename = regexp.MustCompile(`^(\d{6})_(.+)\.up\.sql$`)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	result := make([]Migration, 0, len(entries))
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		match := migrationFilename.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		if _, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %06d", version)
		}
		seen[version] = struct{}{}

		sql, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		result = append(result, Migration{
			Version: version,
			Name:    match[2],
			SQL:     string(sql),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})
	for i, migration := range result {
		expected := i + 1
		if migration.Version != expected {
			return nil, fmt.Errorf("migration sequence gap: expected %06d, got %06d", expected, migration.Version)
		}
	}
	return result, nil
}

func RunMigrations(ctx context.Context, databaseURL string, appliedAt time.Time) (int, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return 0, fmt.Errorf("database URL is required")
	}

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return 0, fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	loaded, err := LoadMigrations()
	if err != nil {
		return 0, err
	}

	if _, err := conn.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version    BIGINT PRIMARY KEY,
            name       TEXT NOT NULL,
            applied_at TIMESTAMPTZ NOT NULL
        )
    `); err != nil {
		return 0, fmt.Errorf("create schema_migrations: %w", err)
	}

	appliedCount := 0
	for _, migration := range loaded {
		var appliedName string
		err := conn.QueryRow(ctx,
			"SELECT name FROM schema_migrations WHERE version = $1",
			migration.Version,
		).Scan(&appliedName)
		if err == nil {
			if appliedName != migration.Name {
				return appliedCount, fmt.Errorf(
					"migration %06d name mismatch: database=%q embedded=%q",
					migration.Version, appliedName, migration.Name,
				)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return appliedCount, fmt.Errorf("read migration %06d state: %w", migration.Version, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return appliedCount, fmt.Errorf("begin migration %06d: %w", migration.Version, err)
		}

		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("execute migration %06d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)",
			migration.Version, migration.Name, appliedAt.UTC(),
		); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("record migration %06d: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return appliedCount, fmt.Errorf("commit migration %06d: %w", migration.Version, err)
		}
		appliedCount++
	}

	return appliedCount, nil
}
