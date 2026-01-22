package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate all models
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

	return db
}

// createTestWorkspace creates a workspace for testing
func createTestWorkspace(t *testing.T, db *gorm.DB) uuid.UUID {
	user := models.User{
		ID:    uuid.New(),
		Name:  "Test User",
		Email: "test@example.com",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "Test Workspace",
	}
	require.NoError(t, db.Create(&workspace).Error)

	return workspace.ID
}

// ==================== Agent Handler Tests ====================

func TestAgentHandler_Create(t *testing.T) {
	db := setupTestDB(t)
	workspaceID := createTestWorkspace(t, db)
	handler := NewAgentHandler(db, &MockHub{})

	e := echo.New()

	t.Run("successfully creates agent", func(t *testing.T) {
		reqBody := CreateAgentRequest{
			Name:         "Test Agent",
			ProviderType: "mcp",
			Config: map[string]any{
				"endpoint": "https://example.com",
				"token":    "secret-token",
			},
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(workspaceID.String())

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var response models.Agent
		json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Equal(t, "Test Agent", response.Name)
		assert.Equal(t, "mcp", response.ProviderType)
		assert.True(t, response.Enabled)
	})

	t.Run("fails with invalid workspace_id", func(t *testing.T) {
		reqBody := CreateAgentRequest{Name: "Test", ProviderType: "mcp"}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues("invalid-uuid")

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAgentHandler_List(t *testing.T) {
	db := setupTestDB(t)
	workspaceID := createTestWorkspace(t, db)
	handler := NewAgentHandler(db, &MockHub{})

	// Create some agents
	for i := 0; i < 3; i++ {
		agent := models.Agent{
			ID:           uuid.New(),
			WorkspaceID:  workspaceID,
			Name:         "Agent " + string(rune('A'+i)),
			ProviderType: "mcp",
			Enabled:      true,
			Position:     i,
		}
		db.Create(&agent)
	}

	e := echo.New()

	t.Run("lists all agents in workspace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(workspaceID.String())

		err := handler.List(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var agents []models.Agent
		json.Unmarshal(rec.Body.Bytes(), &agents)
		assert.Len(t, agents, 3)
	})

	t.Run("returns empty for different workspace", func(t *testing.T) {
		differentWS := uuid.New()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(differentWS.String())

		err := handler.List(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var agents []models.Agent
		json.Unmarshal(rec.Body.Bytes(), &agents)
		assert.Len(t, agents, 0) // Empty - tenant isolation works!
	})
}

func TestAgentHandler_Reorder(t *testing.T) {
	db := setupTestDB(t)
	workspaceID := createTestWorkspace(t, db)
	handler := NewAgentHandler(db, &MockHub{})

	// Create agents
	agentIDs := make([]uuid.UUID, 3)
	for i := 0; i < 3; i++ {
		agent := models.Agent{
			ID:           uuid.New(),
			WorkspaceID:  workspaceID,
			Name:         "Agent " + string(rune('A'+i)),
			ProviderType: "mcp",
			Position:     i,
		}
		db.Create(&agent)
		agentIDs[i] = agent.ID
	}

	e := echo.New()

	t.Run("successfully reorders agents", func(t *testing.T) {
		// Reverse the order
		reqBody := ReorderRequest{
			AgentIDs: []string{agentIDs[2].String(), agentIDs[1].String(), agentIDs[0].String()},
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(workspaceID.String())

		err := handler.Reorder(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify new positions
		var agent0, agent2 models.Agent
		db.First(&agent0, "id = ?", agentIDs[0])
		db.First(&agent2, "id = ?", agentIDs[2])
		assert.Equal(t, 2, agent0.Position) // Was first, now last
		assert.Equal(t, 0, agent2.Position) // Was last, now first
	})
}

func TestAgentHandler_SpyPayload(t *testing.T) {
	db := setupTestDB(t)
	workspaceID := createTestWorkspace(t, db)
	handler := NewAgentHandler(db, &MockHub{})

	// Create agent with secrets
	configBytes, _ := json.Marshal(map[string]any{
		"endpoint": "https://example.com",
		"token":    "super-secret-token-12345",
		"api_key":  "sk-abcdef123456",
	})
	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		Name:         "Secret Agent",
		ProviderType: "openai",
		Config:       configBytes,
	}
	db.Create(&agent)

	e := echo.New()

	t.Run("redacts sensitive fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?question=Hello", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(agent.ID.String())

		err := handler.SpyPayload(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var payload map[string]any
		json.Unmarshal(rec.Body.Bytes(), &payload)

		config := payload["config"].(map[string]any)

		// Endpoint should not be redacted
		assert.Equal(t, "https://example.com", config["endpoint"])

		// Token should be redacted
		tokenVal := config["token"].(string)
		assert.Contains(t, tokenVal, "****")
		assert.NotEqual(t, "super-secret-token-12345", tokenVal)

		// API key should be redacted
		apiKeyVal := config["api_key"].(string)
		assert.Contains(t, apiKeyVal, "****")
		assert.NotEqual(t, "sk-abcdef123456", apiKeyVal)
	})
}

func TestAgentHandler_Delete(t *testing.T) {
	db := setupTestDB(t)
	workspaceID := createTestWorkspace(t, db)
	handler := NewAgentHandler(db, &MockHub{})

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		Name:         "To Delete",
		ProviderType: "mcp",
	}
	db.Create(&agent)

	e := echo.New()

	t.Run("successfully deletes agent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(agent.ID.String())

		err := handler.Delete(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)

		// Verify deleted
		var count int64
		db.Model(&models.Agent{}).Where("id = ?", agent.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})
}
