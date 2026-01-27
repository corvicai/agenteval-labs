package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAdminHandlers(t *testing.T) {
	setup()

	t.Run("AccessControl", func(t *testing.T) {
		// Normal user trying to access admin function
		_, token := createTestUser(t, false)
		resp := sendWSRequest(t, token, ReqAdminGetUsers, map[string]string{})
		assert.Equal(t, EvtError, resp.Type)
		var payload map[string]string
		json.Unmarshal(resp.Payload, &payload)
		assert.Contains(t, payload["error"], "admin access required")
	})

	t.Run("GetUsers", func(t *testing.T) {
		_, adminToken := createTestUser(t, true)
		resp := sendWSRequest(t, adminToken, ReqAdminGetUsers, map[string]string{})
		assert.Equal(t, DataAdminUsers, resp.Type)

		var payload []any
		json.Unmarshal(resp.Payload, &payload)
		assert.GreaterOrEqual(t, len(payload), 1)
	})

	t.Run("CreateUser", func(t *testing.T) {
		_, adminToken := createTestUser(t, true)
		newEmail := "new_" + uuid.New().String() + "@example.com"

		payload := map[string]string{
			"name":     "New User",
			"email":    newEmail,
			"password": "password123",
			"role":     "user",
		}

		resp := sendWSRequest(t, adminToken, ReqAdminCreateUser, payload)
		assert.Equal(t, DataAdminUsers, resp.Type)

		var respData map[string]any
		json.Unmarshal(resp.Payload, &respData)
		assert.Equal(t, newEmail, respData["email"])

		// Verify in DB
		var dbUser models.User
		err := db.First(&dbUser, "email = ?", newEmail).Error
		assert.Nil(t, err)
	})

	t.Run("CreateOrg", func(t *testing.T) {
		_, adminToken := createTestUser(t, true)
		orgName := "New Org " + uuid.New().String()

		payload := map[string]string{
			"name": orgName,
		}

		resp := sendWSRequest(t, adminToken, ReqAdminCreateOrg, payload)
		assert.Equal(t, DataAdminOrganizations, resp.Type)

		var respData map[string]any
		json.Unmarshal(resp.Payload, &respData)
		assert.Equal(t, orgName, respData["name"])
	})

	t.Run("GenerateInvite", func(t *testing.T) {
		adminUser, adminToken := createTestUser(t, true)

		// Admin needs to be in an org to generate invite usually, or specify org_id?
		// handleAdminGenerateInvite implementation:
		// checks payload["organization_id"] or uses user's org.

		// Let's create an org for the admin first or use the one created by createTestUser
		var userOrg models.UserOrganization
		db.First(&userOrg, "user_id = ?", adminUser.ID)

		resp := sendWSRequest(t, adminToken, ReqAdminGenerateInvite, map[string]any{
			"target_org_id": userOrg.OrganizationID.String(),
			"max_uses":      5,
		})

		assert.Equal(t, DataResponse, resp.Type)
		var respData map[string]any
		json.Unmarshal(resp.Payload, &respData)
		assert.NotEmpty(t, respData["code"])
	})
}
