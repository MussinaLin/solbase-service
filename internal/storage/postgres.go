package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	log "github.com/sirupsen/logrus"
)

// MigrationsFS holds the embedded migrations filesystem (set by main package).
var MigrationsFS embed.FS

// MigrationsDir is the directory path within the embedded FS.
var MigrationsDir = "migrations"

// Database wraps pgxpool and sqlc queries.
type Database struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
}

// global database instance.
var globalDB *Database

// closeOnce ensures Close() is only executed once.
var closeOnce sync.Once

// Connect initializes the database connection and runs migrations.
func Connect(databaseURL string, logLevel string) (*Database, error) {
	ctx := context.Background()

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	poolConfig.MaxConns = 100
	poolConfig.MinConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("Database connection established successfully")

	if err := RunMigrations(pool); err != nil {
		pool.Close()

		return nil, fmt.Errorf("migrations failed: %w", err)
	}

	queries := db.New(pool)

	database := &Database{
		Pool:    pool,
		Queries: queries,
	}

	globalDB = database

	return database, nil
}

// RunMigrations runs database migrations using goose with embedded migrations.
func RunMigrations(pool *pgxpool.Pool) error {
	log.Info("Running database migrations...")

	sqlDB := stdlib.OpenDBFromPool(pool)

	goose.SetBaseFS(MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(sqlDB, MigrationsDir); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")

	return nil
}

// RunMigrationsWithDB runs migrations with a *sql.DB (for CLI usage).
func RunMigrationsWithDB(sqlDB *sql.DB) error {
	log.Info("Running database migrations...")

	goose.SetBaseFS(MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(sqlDB, MigrationsDir); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")

	return nil
}

// Close closes the database connection pool.
// Safe to call multiple times; only the first call will close the connection.
func Close() {
	closeOnce.Do(func() {
		if globalDB != nil && globalDB.Pool != nil {
			globalDB.Pool.Close()
			log.Info("Database connection closed")
		}
	})
}

// Ping checks if the database connection is alive.
func Ping() error {
	if globalDB == nil || globalDB.Pool == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := globalDB.Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// DB returns the global database instance.
func DB() *Database {
	return globalDB
}
