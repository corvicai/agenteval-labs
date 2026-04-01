package api

import (
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteQuestionSetDeletesHistoryAndRelatedData(t *testing.T) {
	setup()

	user, workspace, _, questionSet, token := createQuestionSetTestFixture(t, "delete-owner")

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		Name:         "Primary Agent",
		ProviderType: "mcp",
		Config:       models.EncryptedJSON(`{"endpoint":"https://example.com","token":"secret"}`),
		Enabled:      true,
	}
	require.NoError(t, db.Create(&agent).Error)
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: questionSet.ID,
		AgentID:       agent.ID,
		Enabled:       true,
		Position:      1,
	}).Error)

	shareLink := models.QuestionSetShareLink{
		ID:              uuid.New(),
		Token:           "delete-link-" + uuid.NewString(),
		QuestionSetID:   questionSet.ID,
		CreatedByUserID: user.ID,
		ExpiresAt:       questionSet.CreatedAt.AddDate(0, 0, 7),
	}
	require.NoError(t, db.Create(&shareLink).Error)

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspace.ID,
		QuestionSetID: questionSet.ID,
		Status:        "completed",
	}
	require.NoError(t, db.Create(&run).Error)

	runResult := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    agent.ID,
		QuestionID: "q1",
		Status:     "success",
	}
	require.NoError(t, db.Create(&runResult).Error)

	evaluation := models.Evaluation{
		ID:          uuid.New(),
		RunResultID: runResult.ID,
		RaterType:   "user",
		RaterID:     user.ID,
		Rating:      "like",
	}
	require.NoError(t, db.Create(&evaluation).Error)

	resp := sendWSRequest(t, token, ReqDeleteQuestionSet, map[string]any{
		"id": questionSet.ID.String(),
	})
	assert.Equal(t, DataResponse, resp.Type)

	var payload map[string]any
	decodeWSResponsePayload(t, resp, &payload)
	assert.Equal(t, true, payload["deleted"])
	assert.Equal(t, questionSet.ID.String(), payload["id"])

	assert.Error(t, db.First(&models.QuestionSet{}, "id = ?", questionSet.ID).Error)

	var questionSetAgentCount int64
	require.NoError(t, db.Model(&models.QuestionSetAgent{}).Where("question_set_id = ?", questionSet.ID).Count(&questionSetAgentCount).Error)
	assert.Zero(t, questionSetAgentCount)

	var shareLinkCount int64
	require.NoError(t, db.Model(&models.QuestionSetShareLink{}).Where("question_set_id = ?", questionSet.ID).Count(&shareLinkCount).Error)
	assert.Zero(t, shareLinkCount)

	var runCount int64
	require.NoError(t, db.Model(&models.Run{}).Where("question_set_id = ?", questionSet.ID).Count(&runCount).Error)
	assert.Zero(t, runCount)

	var runResultCount int64
	require.NoError(t, db.Model(&models.RunResult{}).Where("run_id = ?", run.ID).Count(&runResultCount).Error)
	assert.Zero(t, runResultCount)

	var evaluationCount int64
	require.NoError(t, db.Model(&models.Evaluation{}).Where("run_result_id = ?", runResult.ID).Count(&evaluationCount).Error)
	assert.Zero(t, evaluationCount)
}

func TestDeleteQuestionSetBlocksRunningBenchmark(t *testing.T) {
	setup()

	_, workspace, _, questionSet, token := createQuestionSetTestFixture(t, "delete-running")

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspace.ID,
		QuestionSetID: questionSet.ID,
		Status:        "running",
	}
	require.NoError(t, db.Create(&run).Error)

	resp := sendWSRequest(t, token, ReqDeleteQuestionSet, map[string]any{
		"id": questionSet.ID.String(),
	})
	assert.Equal(t, EvtError, resp.Type)

	var payload map[string]any
	decodeWSResponsePayload(t, resp, &payload)
	assert.Equal(t, "cannot delete a question set with a running benchmark", payload["error"])

	var count int64
	require.NoError(t, db.Model(&models.QuestionSet{}).Where("id = ?", questionSet.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
