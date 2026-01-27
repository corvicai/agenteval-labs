package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Println("[DB] DATABASE_URL not set, using default local DSN")
		dsn = "host=localhost user=postgres password=postgres dbname=benchmarking port=5432 sslmode=disable"
	} else {
		log.Println("[DB] Connecting using DATABASE_URL from environment")
	}

	// Retry connection up to 30 times (30 seconds total)
	var db *gorm.DB
	var err error
	maxRetries := 30

	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn), // Default to Warn in prod unless debug needed
		})
		if err == nil {
			// Configure connection pool
			sqlDB, err := db.DB()
			if err == nil {
				sqlDB.SetMaxOpenConns(100)
				sqlDB.SetMaxIdleConns(10)
				sqlDB.SetConnMaxLifetime(time.Hour)
				log.Println("[DB] Connection pool configured: MaxOpen=100, MaxIdle=10, Lifetime=1h")
			}
			return db, nil
		}

		if i < maxRetries-1 {
			if i%5 == 0 {
				log.Printf("[DB] Still waiting for database... (%d/%d)", i, maxRetries)
			}
			time.Sleep(time.Second)
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
}

func AutoMigrate(db *gorm.DB) error {
	log.Println("[DB] Starting database migrations...")

	// 1. Ensure migrations table exists
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 2. Read migration files
	migrationDir := "migrations"
	files, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	// 3. Filter and sort SQL files
	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && (len(f.Name()) > 4 && f.Name()[len(f.Name())-4:] == ".sql") {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}
	// Note: ReadDir already sorts alphabetically

	// 4. Run migrations
	for _, filename := range sqlFiles {
		// Check if already applied
		var count int64
		db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", filename).Scan(&count)
		if count > 0 {
			continue
		}

		log.Printf("[DB] Applying migration: %s", filename)
		content, err := os.ReadFile(fmt.Sprintf("%s/%s", migrationDir, filename))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", filename, err)
		}

		if err := db.Exec(string(content)).Error; err != nil {
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}

		// Record success
		if err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", filename).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}
	}

	log.Println("[DB] Migrations completed successfully")
	return nil
}
