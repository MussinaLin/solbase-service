package database

import (
	"context"
	"fmt"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

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
	if err := RunMigrations(databaseURL); err != nil {
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

// RunMigrations runs database migrations using golang-migrate
func RunMigrations(databaseURL string) error {
	log.Info("Running database migrations...")

	m, err := migrate.New(
		"file://db/migrations",
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
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
