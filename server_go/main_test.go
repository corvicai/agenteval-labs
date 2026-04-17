package main

import (
	"testing"

	"benchmarking-platform/internal/service"

	"github.com/stretchr/testify/require"
)

func TestShouldBlockStartupForEncryptionHealth(t *testing.T) {
	t.Run("production blocks on mismatch without auto-promote flag", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "mismatch", StateSummary: "fingerprint mismatch"},
			false,
		)
		require.True(t, blocked)
		require.NotEmpty(t, reason)
	})

	t.Run("production does not block on mismatch when auto-promote is set", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "mismatch", StateSummary: "fingerprint mismatch"},
			true,
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})

	t.Run("production blocks on sentinel failure regardless of auto-promote", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "sentinel_failed", StateSummary: "sentinel failed"},
			true,
		)
		require.True(t, blocked)
		require.Equal(t, "sentinel failed", reason)
	})

	t.Run("production does not block on match", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "match"},
			false,
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})

	t.Run("non production does not block regardless of status", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"development",
			service.EncryptionKeyHealth{StateStatus: "sentinel_failed", StateSummary: "sentinel failed"},
			false,
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})

	t.Run("non production does not block on mismatch either", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"development",
			service.EncryptionKeyHealth{StateStatus: "mismatch"},
			false,
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})
}
