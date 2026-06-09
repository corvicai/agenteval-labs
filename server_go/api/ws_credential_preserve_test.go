package api

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"benchmarking-platform/models"
)

// The UI shows redacted secrets ("sk****yz"). Saving the agent must never
// persist that mask over the real stored credential.
func TestHandleUpdateAgent_PreservesMaskedSecret(t *testing.T) {
	setup()

	owner, token := createTestUser(t, false)
	ws := models.Workspace{ID: uuid.New(), UserID: owner.ID, Name: "WS"}
	require.NoError(t, db.Create(&ws).Error)
	agent := models.Agent{
		ID: uuid.New(), WorkspaceID: ws.ID, Name: "BMW", ProviderType: "openai",
		Enabled: true, MaxConcurrency: 5,
		Config: models.EncryptedJSON(`{"api_key":"sk-realsecret-123456","model":"gpt-4o-mini"}`),
	}
	require.NoError(t, db.Create(&agent).Error)

	resp := sendWSRequest(t, token, ReqUpdateAgent, map[string]any{
		"id": agent.ID.String(), "name": "BMW", "provider_type": "openai", "enabled": true,
		"config": map[string]any{"api_key": "sk****56", "model": "gpt-4o"},
	})
	require.Equal(t, DataResponse, resp.Type, "payload: %s", string(resp.Payload))

	var reloaded models.Agent
	require.NoError(t, db.First(&reloaded, "id = ?", agent.ID).Error)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(reloaded.Config, &cfg))
	assert.Equal(t, "sk-realsecret-123456", cfg["api_key"], "masked key must not overwrite the real one")
	assert.Equal(t, "gpt-4o", cfg["model"], "non-secret field should still update")
}

// The per-question-set override (QuestionSetAgent.Config) merges over the base
// at run time. A masked secret saved here silently shadows the real key with
// "sk****yz" -> 401 at run time. Saving must preserve the base secret.
func TestHandleUpdateQuestionSetAgents_PreservesMaskedOverrideSecret(t *testing.T) {
	setup()

	owner, token := createTestUser(t, false)
	ws := models.Workspace{ID: uuid.New(), UserID: owner.ID, Name: "WS"}
	require.NoError(t, db.Create(&ws).Error)
	client := models.Client{ID: uuid.New(), WorkspaceID: ws.ID, Name: "C"}
	require.NoError(t, db.Create(&client).Error)
	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: []byte(`{}`)}
	require.NoError(t, db.Create(&qs).Error)
	agent := models.Agent{
		ID: uuid.New(), WorkspaceID: ws.ID, Name: "BMW", ProviderType: "openai",
		Enabled: true, MaxConcurrency: 5,
		Config: models.EncryptedJSON(`{"api_key":"sk-realsecret-123456","model":"gpt-4o-mini"}`),
	}
	require.NoError(t, db.Create(&agent).Error)

	resp := sendWSRequest(t, token, ReqUpdateQuestionSetAgents, map[string]any{
		"question_set_id": qs.ID.String(),
		"agents": []map[string]any{
			{
				"agent_id": agent.ID.String(), "enabled": true, "position": 0,
				"config": map[string]any{"api_key": "sk****56", "model": "gpt-4o"},
			},
		},
	})
	require.NotEqual(t, EvtError, resp.Type, "payload: %s", string(resp.Payload))

	var override models.QuestionSetAgent
	require.NoError(t, db.First(&override, "question_set_id = ? AND agent_id = ?", qs.ID, agent.ID).Error)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(override.Config, &cfg))
	assert.Equal(t, "sk-realsecret-123456", cfg["api_key"], "masked override key must not overwrite the real one")
}
