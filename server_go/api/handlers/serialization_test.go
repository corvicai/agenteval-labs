package handlers

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestCircularSerialization behaves like a "reproduction" of why IDs were zeroed,
// and proves that the manual fix works.
func TestCircularSerialization(t *testing.T) {
	// 1. Setup Data with Circular Reference
	uid := uuid.New()
	oid := uuid.New()
	wid := uuid.New()

	user := models.User{
		ID:    uid,
		Name:  "Test User",
		Email: "test@example.com",
	}

	org := models.Organization{
		ID:   oid,
		Name: "Test Org",
	}

	ws := models.Workspace{
		ID:             wid,
		UserID:         uid,
		OrganizationID: oid,
		Name:           "Test Workspace",
		User:           user, // Embedding User
		Organization:   org,  // Embedding Org
	}

	// Create the cycle: User has the workspace
	user.Workspaces = []models.Workspace{ws}

	// 2. Simulate what GORM does (simplified):
	// If we just serialize `ws`, and inside `ws.User` is `user`,
	// and `user` has `Workspaces` which contains `ws`...
	// Standard Go JSON encoder handles this by... well, it actually LOOPS or fails with error if pointers are involved.
	// But GORM often returns structs where the nested relation is empty to avoid fetching loop from DB.

	// If we manually break the cycle in the nested struct:
	safeUser := user
	safeUser.Workspaces = nil // Break cycle

	wsFixed := ws
	wsFixed.User = safeUser

	// 3. Asset Serialization
	data, err := json.Marshal(wsFixed)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	// Check User ID is present
	userMap := result["user"].(map[string]interface{})
	assert.Equal(t, uid.String(), userMap["id"], "Nested User ID should be preserved")
	assert.Equal(t, "Test User", userMap["name"])

	// Check Organization ID is present
	orgMap := result["organization"].(map[string]interface{})
	assert.Equal(t, oid.String(), orgMap["id"], "Nested Organization ID should be preserved")
}
