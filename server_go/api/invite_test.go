package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// Invite Code System Tests
// Run with: go test ./tests/... -v -run TestInviteSystem

func TestInviteSystem(t *testing.T) {
	setup()
	// No teardown needed as we clear DB at start of setup
	// defer teardown()

	// 1. Setup Initial Data (Admin & Manager)
	_, adminToken := createTestUser(t, true)

	// Create Org and Manager
	managerUser := models.User{
		ID:           uuid.New(),
		Name:         "Manager Mike",
		Email:        "mike@test.com",
		PasswordHash: "hash",
	}
	db.Create(&managerUser)

	org := models.Organization{
		ID:        uuid.New(),
		Name:      "Test Org 1",
		ManagerID: &managerUser.ID,
	}
	db.Create(&org)

	db.Create(&models.UserOrganization{
		UserID:         managerUser.ID,
		OrganizationID: org.ID,
		Role:           "manager",
	})

	managerToken := generateTestToken(managerUser.ID, uuid.Nil, org.ID)

	t.Run("Admin Generate New Org Invite", func(t *testing.T) {
		payload := map[string]any{
			"is_new_org": true,
		}
		resp := sendWSRequest(t, adminToken, "REQ_ADMIN_GENERATE_INVITE", payload)

		var data map[string]any
		json.Unmarshal(resp.Payload, &data)

		code, ok := data["code"].(string)
		assert.True(t, ok, "Should return code")
		assert.NotEmpty(t, code, "Code should not be empty")

		// Verify in DB
		var invite models.InviteCode
		err := db.First(&invite, "code = ?", code).Error
		assert.NoError(t, err)
		assert.True(t, invite.IsNewOrg)
		assert.Equal(t, "manager", invite.Role)
	})

	t.Run("Admin Generate Link Org Invite", func(t *testing.T) {
		payload := map[string]any{
			"target_org_id": org.ID.String(),
			"is_new_org":    false,
		}
		resp := sendWSRequest(t, adminToken, "REQ_ADMIN_GENERATE_INVITE", payload)

		var data map[string]any
		json.Unmarshal(resp.Payload, &data)

		code, ok := data["code"].(string)
		assert.True(t, ok)

		var invite models.InviteCode
		err := db.First(&invite, "code = ?", code).Error
		assert.NoError(t, err)
		assert.False(t, invite.IsNewOrg)
		assert.Equal(t, org.ID, *invite.OrganizationID)
		assert.Equal(t, "member", invite.Role)
	})

	t.Run("Manager Generate Invite", func(t *testing.T) {
		resp := sendWSRequest(t, managerToken, "REQ_MANAGER_GENERATE_INVITE", nil)

		var data map[string]any
		json.Unmarshal(resp.Payload, &data)

		code, ok := data["code"].(string)
		assert.True(t, ok)

		var invite models.InviteCode
		err := db.First(&invite, "code = ?", code).Error
		assert.NoError(t, err)
		assert.Equal(t, org.ID, *invite.OrganizationID, "Should be linked to manager's org")
	})

	t.Run("Register with New Org Invite", func(t *testing.T) {
		// Generate code first
		payload := map[string]any{"is_new_org": true}
		resp := sendWSRequest(t, adminToken, "REQ_ADMIN_GENERATE_INVITE", payload)
		var data map[string]any
		json.Unmarshal(resp.Payload, &data)
		code := data["code"].(string)

		// Register
		regPayload := map[string]any{
			"name":              "New Boss",
			"email":             "boss@neworg.com",
			"password":          "password",
			"invite_code":       code,
			"organization_name": "Brand New Corp",
		}

		regResp := sendWSRequest(t, "", "REQ_WS_REGISTER", regPayload)
		var regData map[string]any
		json.Unmarshal(regResp.Payload, &regData)

		assert.Equal(t, true, regData["success"])

		// Verify
		var user models.User
		db.Where("email = ?", "boss@neworg.com").First(&user)
		assert.NotEmpty(t, user.ID)

		var newOrg models.Organization
		db.Where("name = ?", "Brand New Corp").First(&newOrg)
		assert.NotEmpty(t, newOrg.ID)

		var uo models.UserOrganization
		db.Where("user_id = ? AND organization_id = ?", user.ID, newOrg.ID).First(&uo)
		assert.Equal(t, "manager", uo.Role)
	})

	t.Run("Register with Link Org Invite", func(t *testing.T) {
		// Generate code by Manager
		resp := sendWSRequest(t, managerToken, "REQ_MANAGER_GENERATE_INVITE", nil)
		var data map[string]any
		json.Unmarshal(resp.Payload, &data)
		code := data["code"].(string)

		// Register
		regPayload := map[string]any{
			"name":        "New Employee",
			"email":       "emp@test.com",
			"password":    "password",
			"invite_code": code,
		}

		regResp := sendWSRequest(t, "", "REQ_WS_REGISTER", regPayload)
		var regData map[string]any
		json.Unmarshal(regResp.Payload, &regData)

		assert.Equal(t, true, regData["success"])

		// Verify
		var user models.User
		db.Where("email = ?", "emp@test.com").First(&user)

		var uo models.UserOrganization
		db.Where("user_id = ? AND organization_id = ?", user.ID, org.ID).First(&uo)
		assert.Equal(t, "member", uo.Role)
	})
}
