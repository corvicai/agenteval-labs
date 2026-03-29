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

func TestHandleGetLatestRunByQuestionSet_ReturnsLatestLogicalResultOnly(t *testing.T) {
	setup()

	user := models.User{
		ID:           uuid.New(),
		Name:         "Tester",
		Email:        "latest-run@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "WS",
	}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Client",
	}
	require.NoError(t, db.Create(&client).Error)

	questionSet := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     []byte(`{"categories":[{"questions":[{"id":"q-1","question":"What is 2+2?"}]}]}`),
	}
	require.NoError(t, db.Create(&questionSet).Error)

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspace.ID,
		QuestionSetID: questionSet.ID,
		Status:        "completed",
		TotalTasks:    1,
	}
	require.NoError(t, db.Create(&run).Error)

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		Name:         "Primary",
		ProviderType: "openai",
		Config:       models.EncryptedJSON([]byte(`{"api_key":"MOCK"}`)),
		Enabled:      true,
	}
	require.NoError(t, db.Create(&agent).Error)

	now := time.Now().UTC()
	older := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    agent.ID,
		QuestionID: "q-1",
		Status:     "error",
		Error:      "timeout",
		CreatedAt:  now.Add(-time.Minute),
	}
	newer := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    agent.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "4",
		CreatedAt:  now,
	}
	require.NoError(t, db.Create(&older).Error)
	require.NoError(t, db.Create(&newer).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	conn := &Connection{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Send:        make(chan []byte, 1),
		Done:        make(chan struct{}),
	}

	payload, err := json.Marshal(models.GetLatestRunByQSPayload{QuestionSetID: questionSet.ID.String()})
	require.NoError(t, err)

	hub.handleGetLatestRunByQuestionSet(conn, models.Envelope{
		Type:          ReqGetLatestRunByQS,
		CorrelationID: "corr-1",
		Payload:       payload,
	})

	select {
	case raw := <-conn.Send:
		var env models.Envelope
		require.NoError(t, json.Unmarshal(raw, &env))
		var response models.RunLiteResponse
		require.NoError(t, json.Unmarshal(env.Payload, &response))
		require.Len(t, response.Results, 1)
		assert.Equal(t, newer.ID, response.Results[0].ID)
		assert.Equal(t, "success", response.Results[0].Status)
	default:
		t.Fatal("expected websocket response")
	}
}

func TestHandleGetLatestRunByQuestionSet_ReturnsEmptyPayloadWhenNoCompletedRunExists(t *testing.T) {
	setup()

	user := models.User{
		ID:           uuid.New(),
		Name:         "Tester",
		Email:        "latest-empty@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "WS",
	}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Client",
	}
	require.NoError(t, db.Create(&client).Error)

	questionSet := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     []byte(`{"categories":[{"questions":[{"id":"q-1","question":"What is 2+2?"}]}]}`),
	}
	require.NoError(t, db.Create(&questionSet).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	conn := &Connection{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Send:        make(chan []byte, 1),
		Done:        make(chan struct{}),
	}

	payload, err := json.Marshal(models.GetLatestRunByQSPayload{QuestionSetID: questionSet.ID.String()})
	require.NoError(t, err)

	hub.handleGetLatestRunByQuestionSet(conn, models.Envelope{
		Type:          ReqGetLatestRunByQS,
		CorrelationID: "corr-empty",
		Payload:       payload,
	})

	select {
	case raw := <-conn.Send:
		var env models.Envelope
		require.NoError(t, json.Unmarshal(raw, &env))

		var payload map[string]any
		require.NoError(t, json.Unmarshal(env.Payload, &payload))
		assert.Nil(t, payload["run"])
		assert.Nil(t, payload["question_set"])
		assert.NotNil(t, payload["results"])
		assert.NotNil(t, payload["agents"])
	default:
		t.Fatal("expected websocket response")
	}
}
