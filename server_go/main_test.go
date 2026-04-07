package main

import (
	"testing"

	"benchmarking-platform/internal/service"

	"github.com/stretchr/testify/require"
)

func TestShouldBlockStartupForEncryptionHealth(t *testing.T) {
	t.Run("production does not block on mismatch (auto-promotes)", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "mismatch", StateSummary: "fingerprint mismatch"},
			service.EncryptionKeyRotationResult{},
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})

	t.Run("production blocks on sentinel failure", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "sentinel_failed", StateSummary: "sentinel failed"},
			service.EncryptionKeyRotationResult{},
		)
		require.True(t, blocked)
		require.Equal(t, "sentinel failed", reason)
	})

	t.Run("production does not block on match", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"production",
			service.EncryptionKeyHealth{StateStatus: "match"},
			service.EncryptionKeyRotationResult{},
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})

	t.Run("non production does not block", func(t *testing.T) {
		blocked, reason := shouldBlockStartupForEncryptionHealth(
			"development",
			service.EncryptionKeyHealth{StateStatus: "sentinel_failed", StateSummary: "sentinel failed"},
			service.EncryptionKeyRotationResult{},
		)
		require.False(t, blocked)
		require.Empty(t, reason)
	})
}
