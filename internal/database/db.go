package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	log "github.com/sirupsen/logrus"
)

// MigrationsFS holds the embedded migrations filesystem (set by main package)
var MigrationsFS embed.FS

// MigrationsDir is the directory path within the embedded FS
var MigrationsDir string = "db/migrations"

// Database wraps pgxpool and sqlc queries
type Database struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
}

// Global database instance
var DB *Database

// Connect initializes the database connection and runs migrations
func Connect(databaseURL string, logLevel string) (*Database, error) {
	ctx := context.Background()

	// Connect using pgxpool
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure connection pool
	poolConfig.MaxConns = 100
	poolConfig.MinConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("Database connection established successfully")

	// Run migrations
	if err := RunMigrations(pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrations failed: %w", err)
	}

	// Create queries instance
	queries := db.New(pool)

	database := &Database{
		Pool:    pool,
		Queries: queries,
	}

	DB = database
	return database, nil
}

// RunMigrations runs database migrations using goose with embedded migrations
func RunMigrations(pool *pgxpool.Pool) error {
	log.Info("Running database migrations...")

	// Create a *sql.DB from the pgxpool for goose
	sqlDB := stdlib.OpenDBFromPool(pool)

	// Set up goose to use embedded migrations
	goose.SetBaseFS(MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(sqlDB, MigrationsDir); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")
	return nil
}

// RunMigrationsWithDB runs migrations with a *sql.DB (for CLI usage)
func RunMigrationsWithDB(sqlDB *sql.DB) error {
	log.Info("Running database migrations...")

	goose.SetBaseFS(MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(sqlDB, MigrationsDir); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")
	return nil
}

// Close closes the database connection pool
func Close() {
	if DB != nil && DB.Pool != nil {
		DB.Pool.Close()
		log.Info("Database connection closed")
	}
}

// Ping checks if the database connection is alive
func Ping() error {
	if DB == nil || DB.Pool == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := DB.Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
