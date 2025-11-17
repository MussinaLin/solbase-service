package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/models"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Connect initializes the database connection based on the DATABASE_URL
func Connect(databaseURL string, logLevel string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	// Determine database type from connection string
	if strings.HasPrefix(databaseURL, "postgresql://") || strings.HasPrefix(databaseURL, "postgres://") {
		log.Info("Connecting to PostgreSQL database")
		dialector = postgres.Open(databaseURL)
	} else if strings.HasPrefix(databaseURL, "file:") || strings.HasSuffix(databaseURL, ".db") {
		log.Info("Connecting to SQLite database")
		// Remove "file:" prefix if present
		dbPath := strings.TrimPrefix(databaseURL, "file:")
		dialector = sqlite.Open(dbPath)
	} else {
		return nil, fmt.Errorf("unsupported database URL format: %s", databaseURL)
	}

	// Configure GORM logger
	gormLogLevel := logger.Silent
	if logLevel == "debug" {
		gormLogLevel = logger.Info
	}

	// Open database connection
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get generic database object to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Run auto-migration
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("auto-migration failed: %w", err)
	}

	DB = db
	log.Info("Database connection established successfully")

	return db, nil
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	log.Info("Running database migrations...")

	err := db.AutoMigrate(
		&models.PaymentIntent{},
	)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")
	return nil
}

// Close closes the database connection
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}

	log.Info("Database connection closed")
	return nil
}

// Ping checks if the database connection is alive
func Ping() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
