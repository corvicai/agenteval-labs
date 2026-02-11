package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func setupOrchestratorTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Workspace{},
		&models.Client{},
		&models.Agent{},
		&models.QuestionSet{},
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

func setRunnerMode(t *testing.T, mode string) {
	prev := os.Getenv("RUNNER_MODE")
	if err := os.Setenv("RUNNER_MODE", mode); err != nil {
		t.Fatalf("failed to set RUNNER_MODE: %v", err)
	}
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("RUNNER_MODE")
		} else {
			_ = os.Setenv("RUNNER_MODE", prev)
		}
	})
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

func TestEngine_QueueTask(t *testing.T) {
	setRunnerMode(t, "http")
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	// Mock Python server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ExecutionRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := ExecutionResponse{
			RequestID: req.RequestID,
			Success:   true,
			Answer:    "Mocked answer for: " + req.Payload["question"].(string),
			Metadata: map[string]any{
				"duration_ms": 100,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	engine := NewEngine(db, mockServer.URL, 2)
	engine.Start()

	t.Run("executes task and stores result", func(t *testing.T) {
		task := &Task{
			RunID:        runID,
			AgentID:      uuid.New(),
			QuestionID:   "q-1",
			QuestionText: "What is 2+2?",
			AgentConfig:  map[string]any{"endpoint": "test"},
			ProviderType: "mcp",
		}

		engine.QueueTask(task)

		// Wait for processing
		time.Sleep(500 * time.Millisecond)

		// Verify result was stored
		var result models.RunResult
		err := db.First(&result, "question_id = ?", "q-1").Error
		assert.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.Contains(t, result.Answer, "Mocked answer")
	})
}

func TestEngine_CancelRun(t *testing.T) {
	setRunnerMode(t, "http")
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	engine := NewEngine(db, "http://localhost:9999", 1) // Non-existent server
	engine.Start()

	t.Run("cancels run and skips tasks", func(t *testing.T) {
		// Queue several tasks
		for i := 0; i < 5; i++ {
			task := &Task{
				RunID:        runID,
				AgentID:      uuid.New(),
				QuestionID:   "q-" + string(rune('1'+i)),
				QuestionText: "Question " + string(rune('1'+i)),
				ProviderType: "mcp",
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
	setRunnerMode(t, "http")
	db := setupOrchestratorTestDB(t)
	runID, workspaceID := createTestRun(db)

	// Mock Python server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ExecutionResponse{Success: true, Answer: "OK"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	engine := NewEngine(db, mockServer.URL, 1)

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
			ProviderType: "mcp",
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
	setRunnerMode(t, "http")
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	// Mock server that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ExecutionResponse{
			Success: false,
			Error:   "Connection timeout",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	engine := NewEngine(db, mockServer.URL, 1)
	engine.Start()

	t.Run("stores error results", func(t *testing.T) {
		task := &Task{
			RunID:        runID,
			AgentID:      uuid.New(),
			QuestionID:   "q-error",
			QuestionText: "Error test",
			ProviderType: "mcp",
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
	setRunnerMode(t, "http")
	db := setupOrchestratorTestDB(t)
	runID, _ := createTestRun(db)

	var processed int
	var mu sync.Mutex

	// Mock server that tracks concurrent requests
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		processed++
		mu.Unlock()

		time.Sleep(50 * time.Millisecond) // Simulate work

		resp := ExecutionResponse{Success: true, Answer: "OK"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	engine := NewEngine(db, mockServer.URL, 5) // 5 workers
	engine.Start()

	t.Run("processes tasks concurrently", func(t *testing.T) {
		// Queue 10 tasks
		for i := 0; i < 10; i++ {
			task := &Task{
				RunID:        runID,
				AgentID:      uuid.New(),
				QuestionID:   "q-concurrent-" + string(rune('0'+i)),
				QuestionText: "Concurrent test",
				ProviderType: "mcp",
			}
			engine.QueueTask(task)
		}

		// Wait for all to complete
		time.Sleep(1 * time.Second)

		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 10, processed)
	})
}
