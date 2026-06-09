package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

type blockingRunner struct {
	startedCh chan struct{}
	releaseCh chan struct{}

	mu      sync.Mutex
	started int
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{
		startedCh: make(chan struct{}, 8),
		releaseCh: make(chan struct{}),
	}
}

func (r *blockingRunner) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error) {
	r.mu.Lock()
	r.started++
	r.mu.Unlock()

	select {
	case r.startedCh <- struct{}{}:
	default:
	}

	select {
	case <-r.releaseCh:
		return ExecutionResponse{
			Success: true,
			Answer:  "ok",
		}, nil
	case <-ctx.Done():
		return ExecutionResponse{
			Success: false,
			Error:   ctx.Err().Error(),
		}, ctx.Err()
	}
}

func (r *blockingRunner) Health() error {
	return nil
}

func (r *blockingRunner) StartedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

func setupOrchestratorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, os.Setenv("ENCRYPTION_KEY", "1234567890abcdef"))

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

	engine := NewEngine(db, 2, 0)
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

	engine := NewEngine(db, 1, 0)
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

func TestEngine_CancelRun_SkipsTasksAlreadyDequeuedButWaitingSemaphore(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID := createTestRun(db)

	engine := NewEngine(db, 3, 0)
	mockRunner := newBlockingRunner()
	engine.runner = mockRunner
	engine.Start()

	sharedAgentID := uuid.New()

	for i := 0; i < 3; i++ {
		task := &Task{
			RunID:          runID,
			WorkspaceID:    workspaceID,
			AgentID:        sharedAgentID,
			QuestionID:     fmt.Sprintf("q-block-%d", i),
			QuestionText:   "Question",
			ProviderType:   "openai",
			AgentConfig:    mockOpenAIConfig(),
			MaxConcurrency: 1, // Forces additional workers to block on semaphore.
		}
		engine.QueueTask(task)
	}

	select {
	case <-mockRunner.startedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first task did not start in time")
	}

	engine.CancelRun(runID)
	close(mockRunner.releaseCh)

	assert.Eventually(t, func() bool {
		return mockRunner.StartedCount() == 1
	}, 2*time.Second, 50*time.Millisecond, "expected only the already-running task to execute")

	var resultCount int64
	require.NoError(t, db.Model(&models.RunResult{}).Where("run_id = ?", runID).Count(&resultCount).Error)
	assert.LessOrEqual(t, resultCount, int64(1))
}

func TestEngine_RerunTaskMarksRunRunningAndIncrementsTotalTasks(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")
	agent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())

	require.NoError(t, db.Model(&models.Run{}).Where("id = ?", runID).Updates(map[string]any{
		"status":      "completed",
		"total_tasks": 1,
	}).Error)

	engine := NewEngine(db, 0, 0)

	err := engine.RerunTask(runID, agent.ID, "q-1", nil)
	require.NoError(t, err)

	var run models.Run
	require.NoError(t, db.First(&run, "id = ?", runID).Error)
	assert.Equal(t, "running", run.Status)
	assert.Equal(t, 2, run.TotalTasks)
	assert.Equal(t, 1, len(engine.taskQueue))
}

func TestEngine_RerunTaskForEvaluatorKeepsCanonicalEvalQuestionID(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primary := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluator := createTestAgent(t, db, workspaceID, "Judge", "evaluator", map[string]any{
		"llm_provider":    "openai",
		"openai_mode":     "standard",
		"openai_api_key":  "MOCK",
		"target_agent_id": primary.ID.String(),
	})

	targetResult := models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primary.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "latest answer",
	}
	require.NoError(t, db.Create(&targetResult).Error)

	engine := NewEngine(db, 0, 0)

	err := engine.RerunTask(runID, evaluator.ID, "q-1", &RerunTaskOptions{
		ResultID:         targetResult.ID.String(),
		OriginalQuestion: "What is 2+2?",
		ExpectedAnswer:   "4",
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(engine.taskQueue))

	task := <-engine.taskQueue
	assert.Equal(t, fmt.Sprintf("eval-%s-%s", primary.ID, "q-1"), task.QuestionID)
	assert.Equal(t, targetResult.ID, task.TargetRunResultID)
	assert.Equal(t, "latest answer", task.AgentAnswer)
}

func TestEngine_RerunTaskForEvaluatorUsesFrontendExpectedOverrideWithoutOriginalQuestion(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "stale expected")

	primary := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluator := createTestAgent(t, db, workspaceID, "Judge", "evaluator", map[string]any{
		"llm_provider":    "openai",
		"openai_mode":     "standard",
		"openai_api_key":  "MOCK",
		"target_agent_id": primary.ID.String(),
	})

	targetResult := models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primary.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "latest answer",
	}
	require.NoError(t, db.Create(&targetResult).Error)

	engine := NewEngine(db, 0, 0)

	err := engine.RerunTask(runID, evaluator.ID, "q-1", &RerunTaskOptions{
		ResultID:       targetResult.ID.String(),
		ExpectedAnswer: "fresh expected",
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(engine.taskQueue))

	task := <-engine.taskQueue
	assert.Equal(t, fmt.Sprintf("eval-%s-%s", primary.ID, "q-1"), task.QuestionID)
	assert.Equal(t, "What is 2+2?", task.OriginalQuestion)
	assert.Equal(t, "fresh expected", task.ExpectedAnswer)
}

func TestEngine_RerunTaskForEvaluatorUsesTargetAgentFromPrimaryResultID(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primary := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluator := createTestAgent(t, db, workspaceID, "Judge", "evaluator", map[string]any{
		"llm_provider":   "openai",
		"openai_mode":    "standard",
		"openai_api_key": "MOCK",
	})

	targetResult := models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primary.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "latest answer",
	}
	require.NoError(t, db.Create(&targetResult).Error)

	engine := NewEngine(db, 0, 0)

	err := engine.RerunTask(runID, evaluator.ID, "q-1", &RerunTaskOptions{
		ResultID:         targetResult.ID.String(),
		OriginalQuestion: "What is 2+2?",
		ExpectedAnswer:   "4",
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(engine.taskQueue))

	task := <-engine.taskQueue
	assert.Equal(t, fmt.Sprintf("eval-%s-%s", primary.ID, "q-1"), task.QuestionID)
	assert.Equal(t, targetResult.ID, task.TargetRunResultID)
	assert.Equal(t, "latest answer", task.AgentAnswer)
}

func TestEngine_RunEvaluatorsQueuesOnlyLatestLogicalResult(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primary := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluator := createTestAgent(t, db, workspaceID, "Judge", "evaluator", map[string]any{
		"llm_provider":    "openai",
		"openai_mode":     "standard",
		"openai_api_key":  "MOCK",
		"target_agent_id": primary.ID.String(),
	})

	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primary.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "older answer",
		CreatedAt:  now.Add(-time.Minute),
	}).Error)
	require.NoError(t, db.Create(&models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primary.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "latest answer",
		CreatedAt:  now,
	}).Error)

	engine := NewEngine(db, 0, 0)

	err := engine.RunEvaluators(runID, []uuid.UUID{evaluator.ID})
	require.NoError(t, err)

	var run models.Run
	require.NoError(t, db.First(&run, "id = ?", runID).Error)
	assert.Equal(t, 2, run.TotalTasks)
	require.Equal(t, 1, len(engine.taskQueue))

	task := <-engine.taskQueue
	assert.Equal(t, "eval-"+primary.ID.String()+"-q-1", task.QuestionID)
	assert.Equal(t, "latest answer", task.AgentAnswer)
	assert.Equal(t, evaluator.ID, task.AgentID)
}

func TestEngine_StartRun_QueuesQuestionsInAscendingOrder(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)

	user := models.User{ID: uuid.New(), Name: "Test", Email: "ordered@test.com"}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	require.NoError(t, db.Create(&client).Error)

	qsData := map[string]any{
		"categories": []map[string]any{
			{
				"name": "General",
				"questions": []map[string]any{
					{"id": "q-1", "question": "Question 1"},
					{"id": "q-2", "question": "Question 2"},
				},
			},
			{
				"name": "More",
				"questions": []map[string]any{
					{"id": "q-3", "question": "Question 3"},
				},
			},
		},
	}
	qsBytes, err := json.Marshal(qsData)
	require.NoError(t, err)

	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     qsBytes,
	}
	require.NoError(t, db.Create(&qs).Error)

	agentA := createTestAgent(t, db, workspace.ID, "Agent A", "openai", mockOpenAIConfig())
	agentB := createTestAgent(t, db, workspace.ID, "Agent B", "openai", mockOpenAIConfig())

	require.NoError(t, db.Model(&models.Agent{}).Where("id = ?", agentA.ID).Update("max_concurrency", 5).Error)
	require.NoError(t, db.Model(&models.Agent{}).Where("id = ?", agentB.ID).Update("max_concurrency", 7).Error)

	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qs.ID,
		AgentID:       agentB.ID,
		Enabled:       true,
		Position:      0,
	}).Error)
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qs.ID,
		AgentID:       agentA.ID,
		Enabled:       true,
		Position:      1,
	}).Error)

	engine := NewEngine(db, 0, 0)

	run, err := engine.StartRun(workspace.ID, qs.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, 6, run.TotalTasks)
	require.Equal(t, 6, len(engine.taskQueue))

	expected := []struct {
		agentID    uuid.UUID
		questionID string
	}{
		{agentID: agentB.ID, questionID: "q-1"},
		{agentID: agentA.ID, questionID: "q-1"},
		{agentID: agentB.ID, questionID: "q-2"},
		{agentID: agentA.ID, questionID: "q-2"},
		{agentID: agentB.ID, questionID: "q-3"},
		{agentID: agentA.ID, questionID: "q-3"},
	}

	for _, item := range expected {
		task := <-engine.taskQueue
		assert.Equal(t, item.agentID, task.AgentID)
		assert.Equal(t, item.questionID, task.QuestionID)
		assert.Equal(t, 1, task.MaxConcurrency)
	}
}

func TestEngine_StartRun_PreservesExplicitAgentOrder(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)

	user := models.User{ID: uuid.New(), Name: "Test", Email: "explicit-order@test.com"}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	require.NoError(t, db.Create(&client).Error)

	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     []byte(`{"categories":[{"questions":[{"id":"q-1","question":"Question 1"}]}]}`),
	}
	require.NoError(t, db.Create(&qs).Error)

	agentA := createTestAgent(t, db, workspace.ID, "Agent A", "openai", mockOpenAIConfig())
	agentB := createTestAgent(t, db, workspace.ID, "Agent B", "openai", mockOpenAIConfig())

	require.NoError(t, db.Model(&models.Agent{}).Where("id = ?", agentA.ID).Update("position", 10).Error)
	require.NoError(t, db.Model(&models.Agent{}).Where("id = ?", agentB.ID).Update("position", 0).Error)

	engine := NewEngine(db, 0, 0)

	run, err := engine.StartRun(workspace.ID, qs.ID, []uuid.UUID{agentA.ID, agentB.ID})
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, 2, len(engine.taskQueue))

	first := <-engine.taskQueue
	second := <-engine.taskQueue
	assert.Equal(t, agentA.ID, first.AgentID)
	assert.Equal(t, agentB.ID, second.AgentID)
	assert.Equal(t, "q-1", first.QuestionID)
	assert.Equal(t, "q-1", second.QuestionID)
}

func TestEngine_StartRunForUser_PersistsStarter(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)

	user := models.User{ID: uuid.New(), Name: "Starter", Email: "starter-persists@test.com"}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	require.NoError(t, db.Create(&client).Error)

	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "QS",
		Data:     []byte(`{"categories":[{"questions":[{"id":"q-1","question":"Question 1"}]}]}`),
	}
	require.NoError(t, db.Create(&qs).Error)

	agent := createTestAgent(t, db, workspace.ID, "Agent A", "openai", mockOpenAIConfig())

	engine := NewEngine(db, 0, 0)

	run, err := engine.StartRunForUser(workspace.ID, qs.ID, []uuid.UUID{agent.ID}, user.ID)
	require.NoError(t, err)
	require.NotNil(t, run)
	require.NotNil(t, run.CreatedByUserID)
	assert.Equal(t, user.ID, *run.CreatedByUserID)

	var storedRun models.Run
	require.NoError(t, db.First(&storedRun, "id = ?", run.ID).Error)
	require.NotNil(t, storedRun.CreatedByUserID)
	assert.Equal(t, user.ID, *storedRun.CreatedByUserID)
}

func TestEngine_CancelRun_EmitsRunFinishedCancelled(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID := createTestRun(db)

	engine := NewEngine(db, 1, 0)

	eventCh := make(chan map[string]any, 1)
	engine.SetEventCallback(func(wsID uuid.UUID, eventType string, corrID string, payload any) {
		if eventType != "EVT_RUN_FINISHED" {
			return
		}
		if wsID != workspaceID {
			return
		}
		if data, ok := payload.(map[string]any); ok {
			select {
			case eventCh <- data:
			default:
			}
		}
	})

	engine.CancelRun(runID)

	select {
	case payload := <-eventCh:
		assert.Equal(t, runID.String(), payload["run_id"])
		assert.Equal(t, "cancelled", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected EVT_RUN_FINISHED event for cancelled run")
	}
}

func TestEngine_EventCallback(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID := createTestRun(db)

	engine := NewEngine(db, 1, 0)

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

	engine := NewEngine(db, 1, 0)
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
	engine := NewEngine(db, 5, 0) // 5 workers
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
	runID, workspaceID, qsID := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluatorAgent := createTestAgent(t, db, workspaceID, "Evaluator", "evaluator", map[string]any{
		"api_key":         "MOCK",
		"model":           "gpt-4o-mini",
		"openai_mode":     "standard",
		"target_agent_id": primaryAgent.ID.String(),
	})
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       primaryAgent.ID,
		Enabled:       true,
		Position:      0,
	}).Error)
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       evaluatorAgent.ID,
		Enabled:       true,
		Position:      1,
	}).Error)

	engine := NewEngine(db, 2, 0)
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

func TestEngine_StartRun_AutoRunsEvaluatorsAfterPrimaryCompletion(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)
	_, workspaceID, qsID := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluatorAgent := createTestAgent(t, db, workspaceID, "Evaluator", "evaluator", map[string]any{
		"api_key":         "MOCK",
		"model":           "gpt-4o-mini",
		"openai_mode":     "standard",
		"target_agent_id": primaryAgent.ID.String(),
	})
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       primaryAgent.ID,
		Enabled:       true,
		Position:      0,
	}).Error)
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       evaluatorAgent.ID,
		Enabled:       true,
		Position:      1,
	}).Error)

	engine := NewEngine(db, 2, 0)
	engine.Start()

	run, err := engine.StartRun(workspaceID, qsID, []uuid.UUID{primaryAgent.ID})
	require.NoError(t, err)
	require.NotNil(t, run)

	evalQuestionID := fmt.Sprintf("eval-%s-%s", primaryAgent.ID, "q-1")

	assert.Eventually(t, func() bool {
		var count int64
		err := db.Model(&models.RunResult{}).
			Where("run_id = ? AND agent_id = ? AND question_id = ? AND status = ?", run.ID, evaluatorAgent.ID, evalQuestionID, "success").
			Count(&count).Error
		return err == nil && count >= 1
	}, 8*time.Second, 100*time.Millisecond, "expected evaluator result after primary run completion")

	assert.Eventually(t, func() bool {
		var refreshed models.Run
		if err := db.First(&refreshed, "id = ?", run.ID).Error; err != nil {
			return false
		}
		return refreshed.Status == "completed" && refreshed.TotalTasks == 2
	}, 8*time.Second, 100*time.Millisecond, "expected run to finish with evaluator task included")
}

func TestEngine_StartRun_DoesNotAutoRunUnselectedEvaluator(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)
	_, workspaceID, qsID := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluatorAgent := createTestAgent(t, db, workspaceID, "Evaluator", "evaluator", map[string]any{
		"api_key":         "MOCK",
		"model":           "gpt-4o-mini",
		"openai_mode":     "standard",
		"target_agent_id": primaryAgent.ID.String(),
	})

	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID,
		AgentID:       primaryAgent.ID,
		Enabled:       true,
		Position:      0,
	}).Error)

	engine := NewEngine(db, 2, 0)
	engine.Start()

	run, err := engine.StartRun(workspaceID, qsID, []uuid.UUID{primaryAgent.ID})
	require.NoError(t, err)
	require.NotNil(t, run)

	assert.Eventually(t, func() bool {
		var refreshed models.Run
		if err := db.First(&refreshed, "id = ?", run.ID).Error; err != nil {
			return false
		}
		return refreshed.Status == "completed" && refreshed.TotalTasks == 1
	}, 8*time.Second, 100*time.Millisecond, "expected run to finish without evaluator tasks")

	var evaluatorCount int64
	require.NoError(t, db.Model(&models.RunResult{}).
		Where("run_id = ? AND agent_id = ?", run.ID, evaluatorAgent.ID).
		Count(&evaluatorCount).Error)
	assert.Equal(t, int64(0), evaluatorCount)
}

func TestEngine_PersistEvaluatorScore_UpsertsByEvaluator(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	db := setupOrchestratorTestDB(t)
	runID, workspaceID, _ := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	primaryAgent := createTestAgent(t, db, workspaceID, "Primary", "openai", mockOpenAIConfig())
	evaluatorAgent := createTestAgent(t, db, workspaceID, "Evaluator", "evaluator", map[string]any{
		"api_key": "MOCK",
	})

	targetResult := models.RunResult{
		ID:         uuid.New(),
		RunID:      runID,
		AgentID:    primaryAgent.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "Primary answer",
	}
	require.NoError(t, db.Create(&targetResult).Error)

	engine := NewEngine(db, 1, 0)
	task := &Task{
		RunID:             runID,
		AgentID:           evaluatorAgent.ID,
		QuestionID:        fmt.Sprintf("eval-%s-%s", primaryAgent.ID, "q-1"),
		ProviderType:      "evaluator",
		TargetRunResultID: targetResult.ID,
	}

	require.NoError(t, engine.persistEvaluatorScore(task, "Good response. 7/10"))

	var evals []models.Evaluation
	require.NoError(t, db.Where("run_result_id = ? AND rater_type = ?", targetResult.ID, "agent").Find(&evals).Error)
	require.Len(t, evals, 1)
	assert.Equal(t, evaluatorAgent.ID, evals[0].RaterID)
	assert.Equal(t, "valid", evals[0].Rating)
	if assert.NotNil(t, evals[0].RatingCode) {
		assert.Equal(t, 2, *evals[0].RatingCode)
	}
	if assert.NotNil(t, evals[0].Score) {
		assert.Equal(t, 70, *evals[0].Score)
	}

	require.NoError(t, engine.persistEvaluatorScore(task, "Incorrect response. 2/10"))

	evals = nil
	require.NoError(t, db.Where("run_result_id = ? AND rater_type = ?", targetResult.ID, "agent").Find(&evals).Error)
	require.Len(t, evals, 1)
	assert.Equal(t, "dislike", evals[0].Rating)
	if assert.NotNil(t, evals[0].RatingCode) {
		assert.Equal(t, 3, *evals[0].RatingCode)
	}
	if assert.NotNil(t, evals[0].Score) {
		assert.Equal(t, 20, *evals[0].Score)
	}
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

	engine := NewEngine(db, 2, 0)
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

// A config that failed decryption (EncryptedJSON.Scan poison marker) must
// produce an actionable "re-enter credentials" error, not the misleading
// "not configured / set api_key" message — the user DID configure the agent;
// the encryption key changed underneath it.
func TestValidateEvaluatorConfig_DecryptionFailedMarker(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	engine := NewEngine(db, 0, 0)

	evaluator := models.Agent{
		ID:           uuid.New(),
		Name:         "Plasma Evaluator",
		ProviderType: "evaluator",
		Config:       models.EncryptedJSON(`{"_error":"decryption_failed"}`),
	}

	err := engine.validateEvaluatorConfig(evaluator)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be decrypted")
	assert.Contains(t, err.Error(), "Plasma Evaluator")
	assert.NotContains(t, err.Error(), "is not configured")
}

// When the post-run automatic evaluator pass fails (e.g. the evaluator agent is
// misconfigured), the failure must be surfaced to the client via an event, not
// just logged server-side. Otherwise the user sees "the run finished" with no
// evaluations and no explanation of why.
func TestEngine_CheckRunCompletion_EmitsRunErrorWhenAutoEvaluatorFails(t *testing.T) {
	db := setupOrchestratorTestDB(t)
	runID, workspaceID, qsID := createTestRunWithQuestion(db, "q-1", "What is 2+2?", "4")

	// Primary agent (non-evaluator) attached to the question set.
	primary := createTestAgent(t, db, workspaceID, "Primary", "anthropic", mockOpenAIConfig())
	// Evaluator agent with an empty config -> validateEvaluatorConfig fails.
	evaluator := createTestAgent(t, db, workspaceID, "Bad Evaluator", "evaluator", map[string]any{})

	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID, AgentID: primary.ID, Enabled: true, Position: 0,
	}).Error)
	require.NoError(t, db.Create(&models.QuestionSetAgent{
		QuestionSetID: qsID, AgentID: evaluator.ID, Enabled: true, Position: 1,
	}).Error)

	// One completed primary result so the run counts as complete (TotalTasks=1).
	require.NoError(t, db.Create(&models.RunResult{
		ID: uuid.New(), RunID: runID, AgentID: primary.ID, QuestionID: "q-1",
		Status: "success", Answer: "4",
	}).Error)

	engine := NewEngine(db, 0, 0)

	errCh := make(chan map[string]any, 1)
	engine.SetEventCallback(func(wsID uuid.UUID, eventType string, corrID string, payload any) {
		if eventType != "EVT_RUN_ERROR" {
			return
		}
		if data, ok := payload.(map[string]any); ok {
			select {
			case errCh <- data:
			default:
			}
		}
	})

	engine.checkRunCompletion(runID)

	select {
	case payload := <-errCh:
		assert.Equal(t, runID.String(), payload["run_id"])
		msg, _ := payload["error"].(string)
		assert.Contains(t, msg, "Bad Evaluator", "error event should carry the real failure reason")
	case <-time.After(2 * time.Second):
		t.Fatal("expected EVT_RUN_ERROR when the automatic evaluator pass fails")
	}
}
