package orchestrator

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

func setupOrchestratorTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Workspace{},
		&models.Client{},
		&models.Agent{},
		&models.QuestionSet{},
		&models.QuestionSetAgent{},
		&models.Run{},
		&models.RunResult{},
		&models.Evaluation{},
	)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	return db
}

func mockOpenAIConfig() map[string]any {
	return map[string]any{
		"api_key": "MOCK",
		"model":   "gpt-4o-mini",
	}
}

func createTestRun(db *gorm.DB) (uuid.UUID, uuid.UUID) {
	user := models.User{ID: uuid.New(), Name: "Test", Email: "test@test.com"}
	db.Create(&user)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	db.Create(&workspace)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	db.Create(&client)

	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: []byte(`{}`)}
	db.Create(&qs)

	run := models.Run{ID: uuid.New(), WorkspaceID: workspace.ID, QuestionSetID: qs.ID, Status: "running"}
	db.Create(&run)

	return run.ID, workspace.ID
}

func createTestRunWithQuestion(db *gorm.DB, questionID, questionText, expectedAnswer string) (uuid.UUID, uuid.UUID, uuid.UUID) {
	user := models.User{ID: uuid.New(), Name: "Test", Email: "test@test.com"}
	db.Create(&user)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	db.Create(&workspace)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	db.Create(&client)

	questionSetPayload := map[string]any{
		"categories": []map[string]any{
			{
				"name": "General",
				"questions": []map[string]any{
					{
						"id":       questionID,
						"question": questionText,
						"expected": expectedAnswer,
					},
				},
			},
		},
	}
	qsBytes, _ := json.Marshal(questionSetPayload)
	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     qsBytes,
	}
	db.Create(&qs)

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspace.ID,
		QuestionSetID: qs.ID,
		Status:        "running",
		TotalTasks:    1,
	}
	db.Create(&run)

	return run.ID, workspace.ID, qs.ID
}

func createTestAgent(t *testing.T, db *gorm.DB, workspaceID uuid.UUID, name, providerType string, cfg map[string]any) models.Agent {
	t.Helper()

	cfgBytes, err := json.Marshal(cfg)
	require.NoError(t, err)

	agent := models.Agent{
		ID:             uuid.New(),
		WorkspaceID:    workspaceID,
		Name:           name,
		ProviderType:   providerType,
		Enabled:        true,
		MaxConcurrency: 1,
		Config:         models.EncryptedJSON(cfgBytes),
	}

	require.NoError(t, db.Create(&agent).Error)
	return agent
}

func TestEngine_QueueTask(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	engine := NewEngine(db, 2)
	engine.Start()

	t.Run("executes task and stores result", func(t *testing.T) {
		task := &Task{
			RunID:        runID,
			AgentID:      uuid.New(),
			QuestionID:   "q-1",
			QuestionText: "What is 2+2?",
			AgentConfig:  mockOpenAIConfig(),
			ProviderType: "openai",
		}

		engine.QueueTask(task)

		// Wait for processing
		time.Sleep(500 * time.Millisecond)

		// Verify result was stored
		var result models.RunResult
		err := db.First(&result, "question_id = ?", "q-1").Error
		assert.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.NotEmpty(t, result.Answer)
	})
}

func TestEngine_CancelRun(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	engine := NewEngine(db, 1)
	engine.Start()

	t.Run("cancels run and skips tasks", func(t *testing.T) {
		// Queue several tasks
		for i := 0; i < 5; i++ {
			task := &Task{
				RunID:        runID,
				AgentID:      uuid.New(),
				QuestionID:   "q-" + string(rune('1'+i)),
				QuestionText: "Question " + string(rune('1'+i)),
				ProviderType: "openai",
				AgentConfig:  mockOpenAIConfig(),
			}
			engine.QueueTask(task)
		}

		// Cancel immediately
		engine.CancelRun(runID)

		// Verify run status updated
		var run models.Run
		db.First(&run, "id = ?", runID)
		assert.Equal(t, "cancelled", run.Status)
	})
}

func TestEngine_EventCallback(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID := createTestRun(db)

	engine := NewEngine(db, 1)

	var events []string
	var mu sync.Mutex

	engine.SetEventCallback(func(wsID uuid.UUID, eventType string, corrID string, payload any) {
		mu.Lock()
		events = append(events, eventType)
		mu.Unlock()
		assert.Equal(t, workspaceID, wsID)
	})

	engine.Start()

	t.Run("fires events during execution", func(t *testing.T) {
		task := &Task{
			RunID:        runID,
			AgentID:      uuid.New(),
			QuestionID:   "q-event-test",
			QuestionText: "Test",
			ProviderType: "openai",
			AgentConfig:  mockOpenAIConfig(),
		}

		engine.QueueTask(task)

		time.Sleep(500 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		// Should have received TASK_STARTED and TASK_COMPLETED
		assert.Contains(t, events, "EVT_TASK_STARTED")
		assert.Contains(t, events, "EVT_TASK_COMPLETED")
	})
}

func TestEngine_HandlesErrors(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	engine := NewEngine(db, 1)
	engine.Start()

	t.Run("stores error results", func(t *testing.T) {
		task := &Task{
			RunID:        runID,
			AgentID:      uuid.New(),
			QuestionID:   "q-error",
			QuestionText: "Error test",
			ProviderType: "unknown-provider",
		}

		engine.QueueTask(task)

		time.Sleep(500 * time.Millisecond)

		var result models.RunResult
		err := db.First(&result, "question_id = ?", "q-error").Error
		assert.NoError(t, err)
		assert.Equal(t, "error", result.Status)
	})
}

func TestEngine_Concurrency(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)
	engine := NewEngine(db, 5) // 5 workers
	engine.Start()

	t.Run("processes tasks concurrently", func(t *testing.T) {
		// Queue 10 tasks
		for i := 0; i < 10; i++ {
			task := &Task{
				RunID:        runID,
				AgentID:      uuid.New(),
				QuestionID:   fmt.Sprintf("q-concurrent-%d", i),
				QuestionText: "Concurrent test",
				ProviderType: "openai",
				AgentConfig:  mockOpenAIConfig(),
			}
			engine.QueueTask(task)
		}

		assert.Eventually(t, func() bool {
			var count int64
			_ = db.Model(&models.RunResult{}).Where("run_id = ?", runID).Count(&count).Error
			return count == 10
		}, 3*time.Second, 100*time.Millisecond, "expected all queued tasks to be processed")
	})
}

func TestEngine_RetryPrimaryAutoRunsEvaluatorForSameQuestion(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluatorAgent := createTestAgent(t, db, workspaceID, "Evaluator", "evaluator", map[string]any{
		"api_key":         "MOCK",
		"model":           "gpt-4o-mini",
		"openai_mode":     "standard",
		"target_agent_id": primaryAgent.ID.String(),
	})

	engine := NewEngine(db, 2)
	engine.Start()

	engine.QueueTask(&Task{
		RunID:          runID,
		WorkspaceID:    workspaceID,
		AgentID:        primaryAgent.ID,
		QuestionID:     "q-1",
		QuestionText:   "What is 2+2?",
		ExpectedAnswer: "4",
		AgentConfig:    mockOpenAIConfig(),
		ProviderType:   "openai",
		RetryID:        uuid.NewString(),
		MaxConcurrency: 1,
	})

	evalQuestionID := fmt.Sprintf("eval-%s-%s", primaryAgent.ID, "q-1")
	assert.Eventually(t, func() bool {
		var count int64
		err := db.Model(&models.RunResult{}).
			Where("run_id = ? AND agent_id = ? AND question_id = ? AND status = ?", runID, evaluatorAgent.ID, evalQuestionID, "success").
			Count(&count).Error
		return err == nil && count >= 1
	}, 6*time.Second, 100*time.Millisecond, "expected auto evaluator task for retried answer")

	var evalCount int64
	require.NoError(t, db.Model(&models.RunResult{}).
		Where("run_id = ? AND agent_id = ?", runID, evaluatorAgent.ID).
		Count(&evalCount).Error)
	assert.Equal(t, int64(1), evalCount)
}

func TestEngine_StartRun_UsesGlobalCredentialsWhenOverrideConfigIsEmpty(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)
	_, workspaceID, qsID := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())

	emptyOverrideBytes, _ := json.Marshal(map[string]any{})
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       primaryAgent.ID,
		Config:        models.EncryptedJSON(emptyOverrideBytes),
		Enabled:       true,
		Position:      0,
	}).Error)

	engine := NewEngine(db, 2)
	engine.Start()

	startedRun, err := engine.StartRun(workspaceID, qsID, []uuid.UUID{primaryAgent.ID})
	require.NoError(t, err)
	require.NotNil(t, startedRun)

	assert.Eventually(t, func() bool {
		var count int64
		err := db.Model(&models.RunResult{}).
			Where("run_id = ? AND agent_id = ? AND question_id = ? AND status = ?", startedRun.ID, primaryAgent.ID, "q-1", "success").
			Count(&count).Error
		return err == nil && count >= 1
	}, 6*time.Second, 100*time.Millisecond, "expected successful run result even with empty override config")

	// Ensure there is no API-key-missing failure for this question in the run.
	var errorCount int64
	require.NoError(t, db.Model(&models.RunResult{}).
		Where("run_id = ? AND agent_id = ? AND question_id = ? AND status = ?", startedRun.ID, primaryAgent.ID, "q-1", "error").
		Count(&errorCount).Error)
	assert.Equal(t, int64(0), errorCount)

}
