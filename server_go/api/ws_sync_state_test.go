package api

import (
	"testing"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncStateFallsBackWhenAgentConfigCannotBeDecrypted(t *testing.T) {
	setup()

	_, workspace, _, questionSet, token := createQuestionSetTestFixture(t, "sync-fallback-agent")

	badAgentID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO agents (id, workspace_id, name, provider_type, config, enabled, position, max_concurrency, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, badAgentID.String(), workspace.ID.String(), "Broken Agent", "mcp", "Zm9v", true, 1, 5, time.Now()).Error)

	resp := sendWSRequest(t, token, ReqSyncState, map[string]any{})
	assert.Equal(t, DataState, resp.Type)

	var payload models.SyncStatePayload
	decodeWSResponsePayload(t, resp, &payload)

	require.Len(t, payload.Agents, 1)
	assert.Equal(t, badAgentID, payload.Agents[0].ID)
	assert.JSONEq(t, `{}`, string(payload.Agents[0].Config))

	require.Len(t, payload.QuestionSets, 1)
	assert.Equal(t, questionSet.ID, payload.QuestionSets[0].ID)
	assert.NotEmpty(t, payload.Warnings)
}

func TestSyncStateFallsBackWhenQuestionSetAgentOverridesCannotBeDecrypted(t *testing.T) {
	setup()

	_, workspace, _, questionSet, token := createQuestionSetTestFixture(t, "sync-fallback-question-set")

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		Name:         "Healthy Agent",
		ProviderType: "mcp",
		Config:       models.EncryptedJSON(`{"endpoint":"https://example.com","token":"secret"}`),
		Enabled:      true,
		Position:     1,
	}
	require.NoError(t, db.Create(&agent).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO question_set_agents (question_set_id, agent_id, config, enabled, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, questionSet.ID.String(), agent.ID.String(), "Zm9v", true, 1, time.Now()).Error)

	resp := sendWSRequest(t, token, ReqSyncState, map[string]any{})
	assert.Equal(t, DataState, resp.Type)

	var payload models.SyncStatePayload
	decodeWSResponsePayload(t, resp, &payload)

	require.Len(t, payload.QuestionSets, 1)
	assert.Equal(t, questionSet.ID, payload.QuestionSets[0].ID)
	assert.Len(t, payload.QuestionSets[0].Agents, 0)
	assert.NotEmpty(t, payload.Warnings)
}
