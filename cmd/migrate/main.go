package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("migrate failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	dsn := os.Getenv("LEADERBOARD_DB_URL")
	if dsn == "" {
		return fmt.Errorf("LEADERBOARD_DB_URL is required")
	}
	migrationsDir := os.Getenv("LEADERBOARD_MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil { return fmt.Errorf("open db: %w", err) }
	defer db.Close()
	if err := db.PingContext(ctx); err != nil { return fmt.Errorf("ping db: %w", err) }

	if err := ensureSchemaMigrations(ctx, db); err != nil { return fmt.Errorf("ensure schema_migrations: %w", err) }

	files, err := readMigrationFiles(migrationsDir)
	if err != nil { return fmt.Errorf("read migrations: %w", err) }

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil { return fmt.Errorf("get applied: %w", err) }
	for _, f := range files {
		if applied[f] { continue }
		log.Info("applying", "file", f)
		sqlBytes, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil { return fmt.Errorf("read %s: %w", f, err) }
		if err := applyMigration(ctx, db, f, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
		log.Info("applied", "file", f)
	}
	log.Info("done")
	return nil
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version STRING PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	return err
}

func readMigrationFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil { return err }
		if d.IsDir() { return nil }
		name := d.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, name)
		}
		return nil
	})
	if err != nil { return nil, err }
	sort.Strings(files)
	return files, nil
}

func getAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil { return nil, err }
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil { return nil, err }
		m[v] = true
	}
	return m, rows.Err()
}

func applyMigration(ctx context.Context, db *sql.DB, version, sqlText string) error {
	return withTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil { return err }
		_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
		return err
	})
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil { return err }
	defer func() {
		if p := recover(); p != nil { _ = tx.Rollback(); panic(p) }
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
