package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("GetRuns", func(t *testing.T) {
		_, adminToken := createTestUser(t, true)

		starter := models.User{
			ID:           uuid.New(),
			Name:         "Starter User",
			Email:        "starter_" + uuid.New().String() + "@example.com",
			PasswordHash: "hash",
		}
		require.NoError(t, db.Create(&starter).Error)

		owner := models.User{
			ID:           uuid.New(),
			Name:         "Workspace Owner",
			Email:        "owner_" + uuid.New().String() + "@example.com",
			PasswordHash: "hash",
		}
		require.NoError(t, db.Create(&owner).Error)

		workspace := models.Workspace{
			ID:     uuid.New(),
			UserID: owner.ID,
			Name:   "Ops Workspace",
		}
		require.NoError(t, db.Create(&workspace).Error)

		client := models.Client{
			ID:          uuid.New(),
			WorkspaceID: workspace.ID,
			Name:        "Client",
		}
		require.NoError(t, db.Create(&client).Error)

		qs := models.QuestionSet{
			ID:       uuid.New(),
			ClientID: client.ID,
			Name:     "Admin Visibility Set",
			Data:     []byte(`{"categories":[{"questions":[{"id":"q-1","question":"What is 2+2?"}]}]}`),
		}
		require.NoError(t, db.Create(&qs).Error)

		runningRun := models.Run{
			ID:              uuid.New(),
			WorkspaceID:     workspace.ID,
			QuestionSetID:   qs.ID,
			CreatedByUserID: &starter.ID,
			Status:          "running",
			TotalTasks:      5,
			CreatedAt:       time.Now().UTC().Add(-2 * time.Minute),
		}
		completedRun := models.Run{
			ID:            uuid.New(),
			WorkspaceID:   workspace.ID,
			QuestionSetID: qs.ID,
			Status:        "completed_with_errors",
			TotalTasks:    2,
			CreatedAt:     time.Now().UTC().Add(-10 * time.Minute),
		}
		require.NoError(t, db.Create(&runningRun).Error)
		require.NoError(t, db.Create(&completedRun).Error)

		require.NoError(t, db.Create(&models.RunResult{
			ID:         uuid.New(),
			RunID:      runningRun.ID,
			AgentID:    uuid.New(),
			QuestionID: "q-1",
			Status:     "success",
			Answer:     "4",
			CreatedAt:  time.Now().UTC().Add(-90 * time.Second),
		}).Error)
		require.NoError(t, db.Create(&models.RunResult{
			ID:         uuid.New(),
			RunID:      runningRun.ID,
			AgentID:    uuid.New(),
			QuestionID: "q-2",
			Status:     "error",
			Error:      "timeout",
			CreatedAt:  time.Now().UTC().Add(-30 * time.Second),
		}).Error)
		require.NoError(t, db.Create(&models.RunResult{
			ID:         uuid.New(),
			RunID:      completedRun.ID,
			AgentID:    uuid.New(),
			QuestionID: "q-1",
			Status:     "success",
			Answer:     "4",
			CreatedAt:  time.Now().UTC().Add(-9 * time.Minute),
		}).Error)

		resp := sendWSRequest(t, adminToken, ReqAdminGetRuns, map[string]any{"limit": 100})
		assert.Equal(t, DataAdminRuns, resp.Type)

		var payload models.AdminRunsResponse
		require.NoError(t, json.Unmarshal(resp.Payload, &payload))
		require.Len(t, payload.Runs, 2)

		assert.EqualValues(t, 1, payload.Summary.ActiveRuns)
		assert.EqualValues(t, 1, payload.Summary.ActiveWorkspaces)
		assert.EqualValues(t, 1, payload.Summary.ActiveUsers)
		assert.EqualValues(t, 3, payload.Summary.PendingTasks)

		assert.Equal(t, runningRun.ID, payload.Runs[0].ID)
		assert.Equal(t, "Starter User", payload.Runs[0].StartedByName)
		assert.EqualValues(t, 2, payload.Runs[0].ResultCount)
		assert.EqualValues(t, 1, payload.Runs[0].SuccessCount)
		assert.EqualValues(t, 1, payload.Runs[0].ErrorCount)
		assert.EqualValues(t, 3, payload.Runs[0].PendingCount)

		assert.Equal(t, completedRun.ID, payload.Runs[1].ID)
		assert.Equal(t, "Workspace Owner", payload.Runs[1].StartedByName)
	})
}
