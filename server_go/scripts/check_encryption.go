package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"benchmarking-platform/internal/security"
	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// loadDotEnv reads a .env file and sets any unset env vars from it.
func loadDotEnv(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func main() {
	// Load .env if present
	loadDotEnv(".env")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Println("Error: DATABASE_URL not set")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Encryption Diagnostic Tool ===")
	fmt.Printf("Environment: %s\n", os.Getenv("APP_ENV"))

	// 1. Check current key
	rawKey := os.Getenv("ENCRYPTION_KEY")
	if rawKey == "" {
		fmt.Println("[CRITICAL] ENCRYPTION_KEY is NOT set in environment")
	} else {
		key, format, err := security.ParseEncryptionKey(rawKey)
		if err != nil {
			fmt.Printf("[CRITICAL] ENCRYPTION_KEY is invalid: %v\n", err)
		} else {
			fmt.Printf("[OK] Current key loaded (Format: %s, Fingerprint: %s)\n", format, security.KeyFingerprint(key))
		}
	}

	// 2. Check previous key
	rawPrev := os.Getenv("ENCRYPTION_KEY_PREVIOUS")
	if rawPrev != "" {
		key, _, err := security.ParseEncryptionKey(rawPrev)
		if err != nil {
			fmt.Printf("[WARN] ENCRYPTION_KEY_PREVIOUS is invalid: %v\n", err)
		} else {
			fmt.Printf("[OK] Previous key loaded (Fingerprint: %s)\n", security.KeyFingerprint(key))
		}
	} else {
		fmt.Println("[INFO] ENCRYPTION_KEY_PREVIOUS is not set")
	}

	// 3. Reconcile with DB state
	ks := service.NewEncryptionKeyService(db)
	health, err := ks.InspectCurrentKeyHealth()
	if err != nil {
		fmt.Printf("Error inspecting key health: %v\n", err)
	} else {
		fmt.Printf("DB State Status: %s\n", health.StateStatus)
		fmt.Printf("DB State Summary: %s\n", health.StateSummary)
		fmt.Printf("DB Stored Fingerprint Prefix: %s\n", health.StoredFingerprintPrefix)
	}

	// 4. Scan Agents
	fmt.Println("\nScanning Agents...")
	var agents []models.Agent
	db.Find(&agents)

	failing := 0
	for _, a := range agents {
		// Try to decrypt with current keys
		_, _, err := security.DecryptWithConfiguredKeys(string(a.Config))
		if err != nil {
			// Check if it's already plaintext
			trimmed := strings.TrimSpace(string(a.Config))
			if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
				(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				continue
			}
			fmt.Printf("[FAIL] Agent %s (%s) - Decryption failed\n", a.ID, a.Name)
			failing++
		}
	}

	if failing == 0 {
		fmt.Println("[SUCCESS] All existing agents can be decrypted with current configuration.")
	} else {
		fmt.Printf("[ERROR] %d agents are undecryptable. You likely need to provide the correct ENCRYPTION_KEY_PREVIOUS.\n", failing)
	}

	fmt.Println("\n=== Diagnostic Complete ===")
}
