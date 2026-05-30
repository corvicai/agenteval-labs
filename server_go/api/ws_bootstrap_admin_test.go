package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/stretchr/testify/assert"
)

// Regression test for the WebSocket bootstrap-admin privilege escalation:
// REQ_WS_BOOTSTRAP_ADMIN must refuse to mint an admin once one already exists,
// mirroring the REST /auth/bootstrap-admin guard. Without the guard, any
// unauthenticated client can create an admin account at any point in the
// application's lifetime.
func TestWsBootstrapAdmin(t *testing.T) {
	setup()

	bootstrap := func(email string) *models.Envelope {
		// token == "" simulates an unauthenticated client.
		return sendWSRequest(t, "", ReqWsBootstrapAdmin, map[string]string{
			"name":              "Bootstrap " + email,
			"email":             email,
			"password":          "password123",
			"organization_name": "Org " + email,
		})
	}

	t.Run("SucceedsOnFirstRun", func(t *testing.T) {
		resp := bootstrap("founder@example.com")
		assert.Equal(t, DataResponse, resp.Type)

		var count int64
		db.Model(&models.User{}).Where("email = ?", "founder@example.com").Count(&count)
		assert.Equal(t, int64(1), count)
	})

	t.Run("RejectedWhenAdminAlreadyExists", func(t *testing.T) {
		// An admin now exists (created by the first-run subtest). An
		// unauthenticated client must not be able to mint a second admin.
		resp := bootstrap("attacker@example.com")

		assert.Equal(t, EvtError, resp.Type)
		var payload map[string]string
		json.Unmarshal(resp.Payload, &payload)
		assert.Contains(t, payload["error"], "Admin already exists")

		var count int64
		db.Model(&models.User{}).Where("email = ?", "attacker@example.com").Count(&count)
		assert.Equal(t, int64(0), count, "attacker admin must not be created")
	})
}
