package db

import (
	"fmt"
	"os"
	"strings"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func Connect() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Fallback to granular environment variables
		host := os.Getenv("DB_HOST")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbname := os.Getenv("DB_NAME")
		port := os.Getenv("DB_PORT")

		if host != "" && user != "" && dbname != "" {
			if port == "" {
				port = "5432"
			}
			if password == "" {
				// Not providing any password will confuse the gorm connection
				// string parser
				password = "''"
			}
			logger.Info("[DB] Constructing DSN from granular environment variables")
			dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
				host, user, password, dbname, port)
		} else {
			logger.Info("[DB] DATABASE_URL and granular DB vars not set, using default local DSN")
			dsn = "host=localhost user=postgres password=postgres dbname=benchmarking port=5432 sslmode=disable TimeZone=UTC"
		}
	} else {
		logger.Info("[DB] Connecting using DATABASE_URL from environment")
	}

	// Retry connection up to 30 times (30 seconds total)
	var db *gorm.DB
	var err error
	maxRetries := 30

	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		})
		if err == nil {
			// Configure connection pool
			sqlDB, err := db.DB()
			if err == nil {
				sqlDB.SetMaxOpenConns(100)
				sqlDB.SetMaxIdleConns(10)
				sqlDB.SetConnMaxLifetime(time.Hour)
				logger.Info("[DB] Connection pool configured: MaxOpen=100, MaxIdle=10, Lifetime=1h")
			}
			return db, nil
		}

		if i < maxRetries-1 {
			if i%5 == 0 {
				logger.Warn("[DB] Still waiting for database... (%d/%d)", i, maxRetries)
			}
			time.Sleep(time.Second)
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
}

func AutoMigrate(db *gorm.DB) error {
	logger.Info("[DB] Starting database migrations...")

	// 1. Ensure migrations table exists
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
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

		logger.Info("[DB] Applying migration: %s", filename)
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

	logger.Info("[DB] Migrations completed successfully")
	return nil
}

func EnsureCriticalSchema(db *gorm.DB) error {
	logger.Info("[DB] Ensuring critical schema compatibility")

	if err := db.AutoMigrate(
		&models.Run{},
		&models.QuestionSetShareLink{},
	); err != nil {
		return fmt.Errorf("ensure critical schema: %w", err)
	}

	logger.Info("[DB] Critical schema compatibility check completed")
	return nil
}

func IsMissingRunsCreatedByColumnError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, `created_by_user_id`) &&
		strings.Contains(msg, `relation "runs"`) &&
		strings.Contains(msg, `does not exist`)
}

func IsMissingQuestionSetShareLinksRelationError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return (strings.Contains(msg, `relation "question_set_share_links"`) &&
		strings.Contains(msg, `does not exist`)) ||
		strings.Contains(msg, `no such table: question_set_share_links`)
}

func EnsureQuestionSetShareLinkSchema(db *gorm.DB) error {
	logger.Warn("[DB] question_set_share_links missing; attempting on-demand schema repair")
	if err := db.AutoMigrate(&models.QuestionSetShareLink{}); err != nil {
		return fmt.Errorf("ensure question_set_share_links schema: %w", err)
	}
	return nil
}

func IsMissingQuestionSetCollaboratorsRelationError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return (strings.Contains(msg, `relation "question_set_collaborators"`) ||
		strings.Contains(msg, `relation "question_set_collab_invites"`)) &&
		strings.Contains(msg, `does not exist`)
}

func EnsureQuestionSetCollaboratorSchema(db *gorm.DB) error {
	logger.Warn("[DB] question_set_collaborators missing; attempting on-demand schema repair")
	if err := db.AutoMigrate(&models.QuestionSetCollaborator{}, &models.QuestionSetCollabInvite{}); err != nil {
		return fmt.Errorf("ensure question_set_collaborators schema: %w", err)
	}
	return nil
}

func CreateRunCompat(db *gorm.DB, run *models.Run) error {
	if err := db.Create(run).Error; err != nil {
		if !IsMissingRunsCreatedByColumnError(err) {
			return err
		}

		logger.Warn("[DB] runs.created_by_user_id missing; retrying run insert without starter column")
		return db.Omit("CreatedByUserID").Create(run).Error
	}

	return nil
}
