package api

import (
	"encoding/json"
	"testing"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createAgentTestFixture creates a user+workspace+agent tuple and returns a
// signed JWT for that user. Mirrors createQuestionSetTestFixture so the
// collab tests can set up owner/collaborator scenarios quickly.
func createAgentTestFixture(t *testing.T, userName string) (models.User, models.Workspace, models.Agent, string) {
	t.Helper()

	user := models.User{
		ID:           uuid.New(),
		Name:         userName,
		Email:        userName + "_" + uuid.NewString() + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   userName + " Workspace",
	}
	require.NoError(t, db.Create(&workspace).Error)

	agent := models.Agent{
		ID:             uuid.New(),
		WorkspaceID:    workspace.ID,
		Name:           userName + " Agent",
		ProviderType:   "openai",
		Config:         models.EncryptedJSON([]byte(`{"api_key":"SECRET-VALUE-123456","model":"gpt-4o"}`)),
		Enabled:        true,
		MaxConcurrency: 5,
	}
	require.NoError(t, db.Create(&agent).Error)

	token := generateTestToken(user.ID, workspace.ID, uuid.Nil)
	return user, workspace, agent, token
}

// ---------------------------------------------------------------------------
// TestAgentCollab_CreateInvite_OwnerOnly
// ---------------------------------------------------------------------------

func TestAgentCollab_CreateInvite_OwnerOnly(t *testing.T) {
	setup()
	_, _, agent, ownerToken := createAgentTestFixture(t, "agent-invite-owner")
	_, _, _, outsiderToken := createAgentTestFixture(t, "agent-invite-outsider")

	// Owner succeeds.
	resp := sendWSRequest(t, ownerToken, ReqCreateAgentCollabInvite, map[string]any{
		"agent_id":      agent.ID.String(),
		"invited_email": "buddy@example.com",
	})
	require.Equal(t, DataResponse, resp.Type)

	var created struct {
		Token   string `json:"token"`
		AgentID string `json:"agent_id"`
		Role    string `json:"role"`
	}
	decodeWSResponsePayload(t, resp, &created)
	assert.NotEmpty(t, created.Token)
	assert.Equal(t, "user", created.Role)

	var invite models.AgentCollabInvite
	require.NoError(t, db.Where("token = ?", created.Token).First(&invite).Error)
	assert.Equal(t, agent.ID, invite.AgentID)

	// Outsider is rejected with an owner-only message.
	outsiderResp := sendWSRequest(t, outsiderToken, ReqCreateAgentCollabInvite, map[string]any{
		"agent_id": agent.ID.String(),
	})
	require.Equal(t, EvtError, outsiderResp.Type)
	var errPayload map[string]any
	decodeWSResponsePayload(t, outsiderResp, &errPayload)
	assert.Contains(t, errPayload["error"], "owner")
}

// ---------------------------------------------------------------------------
// TestAgentCollab_AcceptInvite_CreatesCollaborator
// ---------------------------------------------------------------------------

func TestAgentCollab_AcceptInvite_CreatesCollaborator(t *testing.T) {
	setup()
	owner, _, agent, ownerToken := createAgentTestFixture(t, "agent-accept-owner")
	collab, _, _, collabToken := createAgentTestFixture(t, "agent-accept-collab")

	createResp := sendWSRequest(t, ownerToken, ReqCreateAgentCollabInvite, map[string]any{
		"agent_id": agent.ID.String(),
	})
	require.Equal(t, DataResponse, createResp.Type)
	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)

	acceptResp := sendWSRequest(t, collabToken, ReqAcceptAgentCollabInvite, map[string]any{
		"token": created.Token,
	})
	require.Equal(t, DataResponse, acceptResp.Type)

	var row models.AgentCollaborator
	require.NoError(t, db.
		Where("agent_id = ? AND user_id = ?", agent.ID, collab.ID).
		First(&row).Error)
	assert.NotNil(t, row.AcceptedAt, "accepted_at must be populated")
	assert.Nil(t, row.RevokedAt, "new collaborator should not be marked revoked")
	assert.Equal(t, owner.ID, row.InvitedByUserID)

	// Invite must be marked used so it can't be reused.
	var invite models.AgentCollabInvite
	require.NoError(t, db.Where("token = ?", created.Token).First(&invite).Error)
	require.NotNil(t, invite.AcceptedAt)

	// Second accept attempt should fail (already used).
	replayResp := sendWSRequest(t, collabToken, ReqAcceptAgentCollabInvite, map[string]any{
		"token": created.Token,
	})
	assert.Equal(t, EvtError, replayResp.Type)
}

// ---------------------------------------------------------------------------
// TestAgentCollab_AcceptInvite_OwnerCannotAcceptOwn
// ---------------------------------------------------------------------------

func TestAgentCollab_AcceptInvite_OwnerCannotAcceptOwn(t *testing.T) {
	setup()
	_, _, agent, ownerToken := createAgentTestFixture(t, "agent-self-accept")

	createResp := sendWSRequest(t, ownerToken, ReqCreateAgentCollabInvite, map[string]any{
		"agent_id": agent.ID.String(),
	})
	require.Equal(t, DataResponse, createResp.Type)
	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)

	resp := sendWSRequest(t, ownerToken, ReqAcceptAgentCollabInvite, map[string]any{
		"token": created.Token,
	})
	require.Equal(t, EvtError, resp.Type)
	var errPayload map[string]any
	decodeWSResponsePayload(t, resp, &errPayload)
	assert.Contains(t, errPayload["error"], "already own")
}

// ---------------------------------------------------------------------------
// TestAgentCollab_ListAndRevoke
// ---------------------------------------------------------------------------

func TestAgentCollab_ListAndRevoke(t *testing.T) {
	setup()
	_, _, agent, ownerToken := createAgentTestFixture(t, "agent-list-owner")
	collab, _, _, collabToken := createAgentTestFixture(t, "agent-list-collab")

	createResp := sendWSRequest(t, ownerToken, ReqCreateAgentCollabInvite, map[string]any{
		"agent_id": agent.ID.String(),
	})
	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)

	sendWSRequest(t, collabToken, ReqAcceptAgentCollabInvite, map[string]any{
		"token": created.Token,
	})

	// Owner lists collaborators → 1 entry.
	listResp := sendWSRequest(t, ownerToken, ReqListAgentCollaborators, map[string]any{
		"agent_id": agent.ID.String(),
	})
	require.Equal(t, DataResponse, listResp.Type)
	var listPayload struct {
		Collaborators []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"collaborators"`
	}
	decodeWSResponsePayload(t, listResp, &listPayload)
	require.Len(t, listPayload.Collaborators, 1)
	assert.Equal(t, collab.ID.String(), listPayload.Collaborators[0].UserID)

	// Revoke.
	revokeResp := sendWSRequest(t, ownerToken, ReqRevokeAgentCollaborator, map[string]any{
		"agent_id": agent.ID.String(),
		"user_id":  collab.ID.String(),
	})
	require.Equal(t, DataResponse, revokeResp.Type)

	// Verify row is soft-deleted.
	var row models.AgentCollaborator
	require.NoError(t, db.
		Where("agent_id = ? AND user_id = ?", agent.ID, collab.ID).
		First(&row).Error)
	require.NotNil(t, row.RevokedAt, "revoked_at should be set after revoke")

	// List should now be empty.
	listAfter := sendWSRequest(t, ownerToken, ReqListAgentCollaborators, map[string]any{
		"agent_id": agent.ID.String(),
	})
	decodeWSResponsePayload(t, listAfter, &listPayload)
	assert.Len(t, listPayload.Collaborators, 0)

	// Second revoke attempt should fail gracefully (already revoked).
	doubleRevoke := sendWSRequest(t, ownerToken, ReqRevokeAgentCollaborator, map[string]any{
		"agent_id": agent.ID.String(),
		"user_id":  collab.ID.String(),
	})
	assert.Equal(t, EvtError, doubleRevoke.Type)
}

// ---------------------------------------------------------------------------
// TestGetAgentAccess_OwnerVsCollaborator
// ---------------------------------------------------------------------------

func TestGetAgentAccess_OwnerVsCollaborator(t *testing.T) {
	setup()
	owner, _, agent, _ := createAgentTestFixture(t, "gaa-owner")
	collab, _, _, _ := createAgentTestFixture(t, "gaa-collab")
	outsider, _, _, _ := createAgentTestFixture(t, "gaa-outsider")

	hub := NewHub(db, nil, "test-secret", nil)

	// Owner.
	access, _, _, err := hub.getAgentAccess(db, owner.ID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agentAccessOwner, access)

	// Outsider — no access.
	access, _, _, err = hub.getAgentAccess(db, outsider.ID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agentAccessNone, access)

	// Add collaborator row directly (skip invite flow).
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.AgentCollaborator{
		ID:              uuid.New(),
		AgentID:         agent.ID,
		UserID:          collab.ID,
		Role:            "user",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}).Error)

	access, _, _, err = hub.getAgentAccess(db, collab.ID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agentAccessUser, access)

	// Revoke → access drops to None.
	require.NoError(t, db.Model(&models.AgentCollaborator{}).
		Where("agent_id = ? AND user_id = ?", agent.ID, collab.ID).
		Update("revoked_at", now).Error)

	access, _, _, err = hub.getAgentAccess(db, collab.ID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agentAccessNone, access)
}

// ---------------------------------------------------------------------------
// TestUpdateAgent_BlocksNonOwner
// ---------------------------------------------------------------------------

func TestUpdateAgent_BlocksNonOwner(t *testing.T) {
	setup()
	_, _, agent, _ := createAgentTestFixture(t, "upd-owner")
	_, _, _, outsiderToken := createAgentTestFixture(t, "upd-outsider")

	resp := sendWSRequest(t, outsiderToken, ReqUpdateAgent, map[string]any{
		"id":            agent.ID.String(),
		"name":          "Hijack",
		"provider_type": "openai",
		"config":        map[string]any{"api_key": "attacker-key"},
		"enabled":       true,
	})
	require.Equal(t, EvtError, resp.Type)
	var errPayload map[string]any
	decodeWSResponsePayload(t, resp, &errPayload)
	assert.Contains(t, errPayload["error"], "owner")

	// Agent should be untouched.
	var after models.Agent
	require.NoError(t, db.First(&after, "id = ?", agent.ID).Error)
	assert.Equal(t, agent.Name, after.Name)
}

// ---------------------------------------------------------------------------
// TestSyncState_IncludesSharedAgent_Redacted
// ---------------------------------------------------------------------------

func TestSyncState_IncludesSharedAgent_Redacted(t *testing.T) {
	setup()
	owner, _, agent, _ := createAgentTestFixture(t, "sync-shared-owner")
	collab, _, _, collabToken := createAgentTestFixture(t, "sync-shared-collab")

	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.AgentCollaborator{
		ID:              uuid.New(),
		AgentID:         agent.ID,
		UserID:          collab.ID,
		Role:            "user",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}).Error)

	resp := sendWSRequest(t, collabToken, ReqSyncState, map[string]any{})
	require.Equal(t, DataState, resp.Type)

	var payload models.SyncStatePayload
	decodeWSResponsePayload(t, resp, &payload)

	require.Len(t, payload.SharedAgents, 1, "collaborator should see one shared agent")
	shared := payload.SharedAgents[0]
	assert.Equal(t, agent.ID, shared.ID)
	assert.Equal(t, owner.ID, shared.OwnerUserID)
	assert.Equal(t, "sync-shared-owner", shared.OwnerName)

	// Config must be redacted — must NOT contain the raw secret.
	raw, _ := json.Marshal(shared.Config)
	assert.NotContains(t, string(raw), "SECRET-VALUE-123456",
		"shared agent config must be redacted, got %s", string(raw))

	// Redaction preserves non-sensitive keys (model).
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(shared.Config, &cfg))
	assert.Equal(t, "gpt-4o", cfg["model"])
	apiKey, _ := cfg["api_key"].(string)
	assert.NotEqual(t, "SECRET-VALUE-123456", apiKey)
	assert.Contains(t, apiKey, "****")
}
