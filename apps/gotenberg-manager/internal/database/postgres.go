package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("✅ Connected to PostgreSQL")
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) RunMigrations(ctx context.Context, migrationsDir string) error {
	// Create migrations tracking table
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Find migration files
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return fmt.Errorf("failed to glob migrations: %w", err)
	}
	sort.Strings(files)

	for _, file := range files {
		basename := filepath.Base(file)

		// Check if already applied
		var count int
		err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = $1", basename).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", basename, err)
		}
		if count > 0 {
			continue
		}

		// Read and execute migration
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", basename, err)
		}

		_, err = db.Pool.Exec(ctx, string(content))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", basename, err)
		}

		// Record migration
		_, err = db.Pool.Exec(ctx, "INSERT INTO schema_migrations (filename) VALUES ($1)", basename)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", basename, err)
		}

		log.Printf("✅ Applied migration: %s", basename)
	}

	return nil
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}
