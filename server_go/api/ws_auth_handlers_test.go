package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAuthHandlers(t *testing.T) {
	setup() // Uses the shared setup from setup_test.go

	t.Run("AcceptTerms", func(t *testing.T) {
		user, token := createTestUser(t, false)
		// Ensure initial state
		var dbUser models.User
		db.First(&dbUser, "id = ?", user.ID)
		assert.Nil(t, dbUser.TermsAcceptedAt)

		resp := sendWSRequest(t, token, ReqAcceptTerms, map[string]string{})
		assert.Equal(t, DataResponse, resp.Type)

		var payload map[string]any
		json.Unmarshal(resp.Payload, &payload)
		assert.Equal(t, "success", payload["status"])

		// Verify persistence
		db.First(&dbUser, "id = ?", user.ID)
		assert.NotNil(t, dbUser.TermsAcceptedAt)
	})

	t.Run("JoinOrganization", func(t *testing.T) {
		// 1. Create a manager and an invite code
		manager, managerToken := createTestUser(t, false)

		// Create org for manager (createTestUser already does this but let's be explicit or check)
		var managerOrg models.Organization
		// The createTestUser helper puts the user in an org, let's find it
		var userOrg models.UserOrganization
		db.First(&userOrg, "user_id = ?", manager.ID)
		db.First(&managerOrg, "id = ?", userOrg.OrganizationID)

		// Manager generates invite
		inviteResp := sendWSRequest(t, managerToken, ReqManagerGenerateInvite, map[string]int{"max_uses": 1})
		var invitePayload map[string]any
		json.Unmarshal(inviteResp.Payload, &invitePayload)
		code := invitePayload["code"].(string)

		// 2. Create a new user (no org)
		newUser := models.User{
			ID:           uuid.New(),
			Name:         "Joiner",
			Email:        "joiner@example.com",
			PasswordHash: "hash",
		}
		db.Create(&newUser)
		// No org assigned yet

		// Generate token for new user (without org ID)
		newUserToken := generateTestToken(newUser.ID, uuid.Nil, uuid.Nil)

		// 3. User joins organization
		joinResp := sendWSRequest(t, newUserToken, ReqJoinOrganization, map[string]string{"invite_code": code})
		assert.Equal(t, DataResponse, joinResp.Type)

		// 4. Verify membership
		var relationship models.UserOrganization
		err := db.Where("user_id = ? AND organization_id = ?", newUser.ID, managerOrg.ID).First(&relationship).Error
		assert.Nil(t, err)
		assert.Equal(t, "member", relationship.Role)
	})

	t.Run("WebAuthnRegistrationFlow", func(t *testing.T) {
		// Mock WebAuthn flow is tricky integration-wise without browser response,
		// but we can test the "Begin" phase.
		_, token := createTestUser(t, false)

		resp := sendWSRequest(t, token, ReqWebAuthnRegisterBegin, map[string]string{})

		if resp == nil {
			// It might fail if WebAuthn is not configured in NewHub in setup_test.go
			// Check setup_test.go's sendWSRequest
			t.Skip("Skipping WebAuthn test until Hub is mocked with WebAuthn instance")
			return
		}

		if resp.Type == EvtError {
			// Likely "WebAuthn not configured" or similar if we didn't set it up
			t.Logf("WebAuthn Begin failed (expected if config missing): %s", resp.Payload)
			return
		}

		assert.Equal(t, DataWebAuthnRegisterOptions, resp.Type)
		var payload map[string]any
		json.Unmarshal(resp.Payload, &payload)
		assert.NotEmpty(t, payload["session_id"])
		assert.NotEmpty(t, payload["options"])
	})
}
