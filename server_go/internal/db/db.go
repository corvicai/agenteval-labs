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
	// Use raw SQL to avoid GORM issues with circular dependencies between models

	// Organizations table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS organizations (
			id UUID PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			is_suspended BOOLEAN DEFAULT false,
			audit_logs_enabled BOOLEAN DEFAULT false,
			manager_id UUID,
			created_by_user_id UUID,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create organizations: %w", err)
	}

	// Users table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			is_admin BOOLEAN DEFAULT false,
			is_suspended BOOLEAN DEFAULT false,
			invited_by_user_id UUID,
			last_login_at TIMESTAMP,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create users: %w", err)
	}

	// Add manager FK (may fail if already exists, that's ok)
	db.Exec(`ALTER TABLE organizations ADD CONSTRAINT fk_org_manager FOREIGN KEY (manager_id) REFERENCES users(id)`)

	// Workspaces table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workspaces (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id),
			organization_id UUID REFERENCES organizations(id),
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create workspaces: %w", err)
	}

	// Clients table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS clients (
			id UUID PRIMARY KEY,
			workspace_id UUID NOT NULL REFERENCES workspaces(id),
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create clients: %w", err)
	}

	// Agents table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id UUID PRIMARY KEY,
			workspace_id UUID NOT NULL REFERENCES workspaces(id),
			name VARCHAR(255) NOT NULL,
			provider_type VARCHAR(50) NOT NULL,
			config JSONB NOT NULL DEFAULT '{}',
			enabled BOOLEAN DEFAULT true,
			position INTEGER DEFAULT 0,
			max_concurrency INTEGER DEFAULT 5,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create agents: %w", err)
	}

	// Question Sets table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS question_sets (
			id UUID PRIMARY KEY,
			client_id UUID NOT NULL REFERENCES clients(id),
			name VARCHAR(255) NOT NULL,
			version VARCHAR(50),
			data JSONB NOT NULL,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create question_sets: %w", err)
	}

	// Runs table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			id UUID PRIMARY KEY,
			workspace_id UUID NOT NULL REFERENCES workspaces(id),
			question_set_id UUID NOT NULL REFERENCES question_sets(id),
			status VARCHAR(50) NOT NULL DEFAULT 'running',
			total_tasks INTEGER DEFAULT 0,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create runs: %w", err)
	}

	// Run Results table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS run_results (
			id UUID PRIMARY KEY,
			run_id UUID NOT NULL REFERENCES runs(id),
			agent_id UUID NOT NULL REFERENCES agents(id),
			question_id VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL,
			answer TEXT,
			error TEXT,
			metadata JSONB DEFAULT '{}',
			duration_ms INTEGER,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create run_results: %w", err)
	}

	// Evaluations table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS evaluations (
			id UUID PRIMARY KEY,
			run_result_id UUID NOT NULL REFERENCES run_results(id),
			rater_type VARCHAR(50) NOT NULL,
			rater_id UUID,
			rating VARCHAR(50) NOT NULL,
			rating_code INTEGER,
			score INTEGER,
			comments TEXT,
			created_at TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create evaluations: %w", err)
	}

	// Stats Cache table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS stats_caches (
			id UUID PRIMARY KEY,
			scope VARCHAR(20) NOT NULL,
			scope_id UUID,
			data JSONB NOT NULL,
			computed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create stats_cache: %w", err)
	}

	// Question Set Agents table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS question_set_agents (
			question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
			agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			enabled BOOLEAN DEFAULT true,
			position INT DEFAULT 0,
			config JSONB,
			created_at TIMESTAMP DEFAULT NOW(),
			PRIMARY KEY (question_set_id, agent_id)
		)
	`).Error; err != nil {
		return fmt.Errorf("create question_set_agents: %w", err)
	}

	// User Organizations table (Many-to-Many)
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS user_organizations (
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			role VARCHAR(50) NOT NULL DEFAULT 'member',
			joined_at TIMESTAMP DEFAULT NOW(),
			PRIMARY KEY (user_id, organization_id)
		)
	`).Error; err != nil {
		return fmt.Errorf("create user_organizations: %w", err)
	}

	// Audit Logs table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id UUID PRIMARY KEY,
			organization_id UUID NOT NULL REFERENCES organizations(id),
			user_id UUID NOT NULL,
			action VARCHAR(100) NOT NULL,
			resource_type VARCHAR(100) NOT NULL,
			resource_id VARCHAR(100),
			details TEXT,
			remote_ip VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create audit_logs: %w", err)
	}

	// Login Logs table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS login_logs (
			id UUID PRIMARY KEY,
			user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			user_email VARCHAR(255) NOT NULL,
			organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
			ip_address VARCHAR(50) NOT NULL,
			user_agent TEXT,
			status VARCHAR(50) NOT NULL,
			failure_reason TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create login_logs: %w", err)
	}

	// Invite Codes table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS invite_codes (
			code VARCHAR(255) PRIMARY KEY,
			created_by UUID NOT NULL REFERENCES users(id),
			organization_id UUID REFERENCES organizations(id),
			role VARCHAR(50) DEFAULT 'member',
			is_new_org BOOLEAN DEFAULT false,
			expires_at TIMESTAMP NOT NULL,
			max_uses INTEGER DEFAULT 1,
			use_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create invite_codes: %w", err)
	}

	// Invite Code Usages table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS invite_code_usages (
			id UUID PRIMARY KEY,
			code VARCHAR(255) NOT NULL REFERENCES invite_codes(code),
			user_id UUID NOT NULL REFERENCES users(id),
			used_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create invite_code_usages: %w", err)
	}

	// Passkeys table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS passkeys (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			credential_id BYTEA UNIQUE NOT NULL,
			public_key BYTEA NOT NULL,
			attestation VARCHAR(50),
			sign_count BIGINT DEFAULT 0,
			backup_eligible BOOLEAN DEFAULT false,
			backup_state BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`).Error; err != nil {
		return fmt.Errorf("create passkeys: %w", err)
	}

	// Apply manual migrations from file if needed (e.g. for new columns on existing tables)
	// For now, let's explicitly add the column here to be safe and simple
	if err := db.Exec(`ALTER TABLE runs ADD COLUMN IF NOT EXISTS total_tasks INTEGER DEFAULT 0;`).Error; err != nil {
		fmt.Printf("Migration warning (total_tasks): %v\n", err)
	}
	db.Exec(`ALTER TABLE organizations ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP;`)
	db.Exec(`ALTER TABLE organizations ADD COLUMN IF NOT EXISTS audit_logs_enabled BOOLEAN DEFAULT false;`)
	db.Exec(`ALTER TABLE organizations ADD COLUMN IF NOT EXISTS created_by_user_id UUID;`)
	db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_suspended BOOLEAN DEFAULT false;`)
	db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP;`)
	db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by_user_id UUID;`)
	db.Exec(`ALTER TABLE passkeys ADD COLUMN IF NOT EXISTS backup_eligible BOOLEAN DEFAULT false;`)
	db.Exec(`ALTER TABLE passkeys ADD COLUMN IF NOT EXISTS backup_state BOOLEAN DEFAULT false;`)
	db.Exec(`ALTER TABLE evaluations ADD COLUMN IF NOT EXISTS rating_code INTEGER;`)
	db.Exec(`ALTER TABLE evaluations ADD COLUMN IF NOT EXISTS score INTEGER;`)
	db.Exec(`
		UPDATE evaluations SET rating_code = CASE
			WHEN rating = 'like' THEN 1
			WHEN rating = 'valid' THEN 2
			WHEN rating = 'dislike' THEN 3
			WHEN rating = 'wrong' THEN 4
			ELSE NULL
		END
		WHERE rating_code IS NULL;
	`)
	db.Exec(`
		UPDATE evaluations SET score = CASE
			WHEN rating_code = 1 THEN 100
			WHEN rating_code = 2 THEN 75
			WHEN rating_code = 3 THEN 25
			WHEN rating_code = 4 THEN 0
			ELSE NULL
		END
		WHERE score IS NULL;
	`)

	db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS max_concurrency INTEGER DEFAULT 5;`)
	db.Exec(`ALTER TABLE run_results ADD COLUMN IF NOT EXISTS error TEXT;`)

	// Bulk Invites migration
	db.Exec(`ALTER TABLE invite_codes ADD COLUMN IF NOT EXISTS max_uses INTEGER DEFAULT 1;`)
	db.Exec(`ALTER TABLE invite_codes ADD COLUMN IF NOT EXISTS use_count INTEGER DEFAULT 0;`)
	// We don't drop used_by/used_at to avoid breaking existing data if any, but they are deprecated.

	return nil
}
