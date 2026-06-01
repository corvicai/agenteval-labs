package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sendWSRequestWithConn(t *testing.T, hub *Hub, conn *Connection, msgType string, payload any) *models.Envelope {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	hub.HandleWSMessage(conn, models.Envelope{
		Type:          msgType,
		CorrelationID: "test-req",
		Payload:       payloadBytes,
	})

	select {
	case respBytes := <-conn.Send:
		var resp models.Envelope
		require.NoError(t, json.Unmarshal(respBytes, &resp))
		return &resp
	default:
		t.Fatal("expected websocket response")
		return nil
	}
}

func seedEvaluationRunResult(t *testing.T, workspaceID, questionSetID uuid.UUID) models.RunResult {
	t.Helper()

	agent := models.Agent{
		ID:             uuid.New(),
		WorkspaceID:    workspaceID,
		Name:           "Primary",
		ProviderType:   "openai",
		Config:         models.EncryptedJSON(`{"api_key":"test"}`),
		Enabled:        true,
		MaxConcurrency: 5,
	}
	require.NoError(t, db.Create(&agent).Error)

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		QuestionSetID: questionSetID,
		Status:        "completed",
		TotalTasks:    1,
	}
	require.NoError(t, db.Create(&run).Error)

	result := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    agent.ID,
		QuestionID: "q1",
		Status:     "success",
		Answer:     "4",
	}
	require.NoError(t, db.Create(&result).Error)
	return result
}

func TestWSKnownProtectedMessagesRequireAuthentication(t *testing.T) {
	setup()

	_, workspace, _, _, _ := createQuestionSetTestFixture(t, "ws-auth-required")
	hub := NewHub(db, nil, "test-secret", nil)
	conn := &Connection{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Send:        make(chan []byte, 2),
		Done:        make(chan struct{}),
	}

	for _, msgType := range []string{ReqSyncState, ReqCreateEvaluation, CmdStartRun} {
		resp := sendWSRequestWithConn(t, hub, conn, msgType, map[string]any{})
		assert.Equal(t, EvtError, resp.Type)
		var payload map[string]string
		decodeWSResponsePayload(t, resp, &payload)
		assert.Contains(t, payload["error"], "authentication required")
	}
}

func TestSyncStateRejectsSpoofedWorkspace(t *testing.T) {
	setup()

	_, victimWorkspace, _, _, _ := createQuestionSetTestFixture(t, "sync-victim")
	attacker, _, _, _, _ := createQuestionSetTestFixture(t, "sync-attacker")

	hub := NewHub(db, nil, "test-secret", nil)
	conn := &Connection{
		ID:              uuid.New(),
		UserID:          attacker.ID,
		WorkspaceID:     victimWorkspace.ID,
		IsAuthenticated: true,
		Send:            make(chan []byte, 1),
		Done:            make(chan struct{}),
	}

	resp := sendWSRequestWithConn(t, hub, conn, ReqSyncState, map[string]any{})
	assert.Equal(t, EvtError, resp.Type)
	var payload map[string]string
	decodeWSResponsePayload(t, resp, &payload)
	assert.Contains(t, payload["error"], "access denied")
}

func TestCreateEvaluationRequiresRunAccessAndReplacesPreviousRating(t *testing.T) {
	setup()

	owner, workspace, _, questionSet, ownerToken := createQuestionSetTestFixture(t, "eval-owner")
	_, _, _, _, attackerToken := createQuestionSetTestFixture(t, "eval-attacker")
	result := seedEvaluationRunResult(t, workspace.ID, questionSet.ID)

	firstResp := sendWSRequest(t, ownerToken, ReqCreateEvaluation, map[string]any{
		"run_result_id": result.ID.String(),
		"rating_code":   1,
	})
	require.Equal(t, DataEvaluation, firstResp.Type)

	secondResp := sendWSRequest(t, ownerToken, ReqCreateEvaluation, map[string]any{
		"run_result_id": result.ID.String(),
		"rating_code":   3,
	})
	require.Equal(t, DataEvaluation, secondResp.Type)

	var evals []models.Evaluation
	require.NoError(t, db.Where("run_result_id = ? AND rater_type = ? AND rater_id = ?", result.ID, "user", owner.ID).Find(&evals).Error)
	require.Len(t, evals, 1)
	require.NotNil(t, evals[0].RatingCode)
	assert.Equal(t, 3, *evals[0].RatingCode)

	attackerResp := sendWSRequest(t, attackerToken, ReqCreateEvaluation, map[string]any{
		"run_result_id": result.ID.String(),
		"rating_code":   1,
	})
	assert.Equal(t, EvtError, attackerResp.Type)

	var attackerCount int64
	require.NoError(t, db.Model(&models.Evaluation{}).
		Where("run_result_id = ? AND rater_type = ? AND rater_id != ?", result.ID, "user", owner.ID).
		Count(&attackerCount).Error)
	assert.Zero(t, attackerCount)
}

func TestStartRunRejectsInvalidAgentID(t *testing.T) {
	setup()

	_, _, _, questionSet, token := createQuestionSetTestFixture(t, "start-invalid-agent")

	resp := sendWSRequest(t, token, CmdStartRun, map[string]any{
		"question_set_id": questionSet.ID.String(),
		"agent_ids":       []string{"not-a-uuid"},
	})

	assert.Equal(t, EvtError, resp.Type)
	var payload map[string]string
	decodeWSResponsePayload(t, resp, &payload)
	assert.Contains(t, payload["error"], "invalid agent_id")
}
