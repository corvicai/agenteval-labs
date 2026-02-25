// Package logger provides levelled logging for the benchmarking platform.
//
// In production (APP_ENV=production) only Warn, Error and Health messages are
// emitted.  In every other environment all levels (Debug, Info, Warn, Error,
// Health) are emitted so that local development retains full visibility.
//
// Usage:
//
//	logger.Init()           // call once in main, before any logging
//	logger.Debug("[WS] ...")
//	logger.Info("[DB] ...")
//	logger.Warn("[SECURITY] ...")
//	logger.Error("[ENGINE] ...")
//	logger.Health("[HEALTH] ...")  // always visible in prod – used by the heartbeat
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type level int

const (
	levelDebug  level = iota // most verbose
	levelInfo                // notable operational events
	levelWarn                // unexpected but non-fatal situations
	levelError               // failures that need attention
	levelHealth              // periodic health/heartbeat – always shown
)

var minLevel level

// Init reads APP_ENV and sets the minimum log level accordingly.
// Call once at programme startup (before any goroutines start logging).
func Init() {
	if strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) == "production" {
		// In production we only want Warn, Error and the heartbeat.
		minLevel = levelWarn
	} else {
		minLevel = levelDebug
	}
}

func emit(lvl level, prefix, format string, args ...any) {
	if lvl < minLevel {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Printf("%s %s", prefix, msg)
}

// Debug logs a message only in non-production environments.
// Use it for high-frequency operational traces (e.g. every WS message).
func Debug(format string, args ...any) {
	emit(levelDebug, "[DEBUG]", format, args...)
}

// Info logs an informational message about notable system events (e.g.
// startup steps, DB connection, Firebase init).
func Info(format string, args ...any) {
	emit(levelInfo, "[INFO]", format, args...)
}

// Warn logs a warning that does not break the system but deserves attention.
func Warn(format string, args ...any) {
	emit(levelWarn, "[WARN]", format, args...)
}

// Error logs a failure that needs attention.  Always visible in every
// environment.
func Error(format string, args ...any) {
	emit(levelError, "[ERROR]", format, args...)
}

// Health logs a periodic heartbeat status message.  It is always emitted
// regardless of APP_ENV so operators know the service is alive.
func Health(format string, args ...any) {
	emit(levelHealth, "[HEALTH]", format, args...)
}

// IsProduction returns true when APP_ENV is "production".
func IsProduction() bool {
	return minLevel >= levelWarn
}
