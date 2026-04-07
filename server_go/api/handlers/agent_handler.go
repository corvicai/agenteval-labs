package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"benchmarking-platform/api"
	"benchmarking-platform/models"
)

type AgentHandler struct {
	DB  *gorm.DB
	Hub api.HubInterface
}

func NewAgentHandler(db *gorm.DB, hub api.HubInterface) *AgentHandler {
	return &AgentHandler{DB: db, Hub: hub}
}

// CreateAgentRequest represents the request body for creating an agent
type CreateAgentRequest struct {
	Name         string         `json:"name" validate:"required"`
	ProviderType string         `json:"provider_type" validate:"required"`
	Config       map[string]any `json:"config"`
	Enabled      *bool          `json:"enabled"`
	Position     *int           `json:"position"`
}

// UpdateAgentRequest represents the request body for updating an agent
type UpdateAgentRequest struct {
	Name         *string        `json:"name"`
	ProviderType *string        `json:"provider_type"`
	Config       map[string]any `json:"config"`
	Enabled      *bool          `json:"enabled"`
	Position     *int           `json:"position"`
}

// ReorderRequest represents the request body for reordering agents
type ReorderRequest struct {
	AgentIDs []string `json:"agent_ids" validate:"required"`
}

// List all agents for a workspace
func (h *AgentHandler) List(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var agents []models.Agent
	if err := h.DB.Where("workspace_id = ?", wsID).Order("position ASC").Find(&agents).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, agents)
}

// Get a single agent
func (h *AgentHandler) Get(c echo.Context) error {
	agentID := c.Param("id")
	id, err := uuid.Parse(agentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
	}

	var agent models.Agent
	if err := h.DB.First(&agent, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, agent)
}

// Create a new agent
func (h *AgentHandler) Create(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var req CreateAgentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Get max position for this workspace
	var maxPos int
	h.DB.Model(&models.Agent{}).Where("workspace_id = ?", wsID).Select("COALESCE(MAX(position), 0)").Scan(&maxPos)

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  wsID,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		Enabled:      true,
		Position:     maxPos + 1,
	}

	if req.Enabled != nil {
		agent.Enabled = *req.Enabled
	}
	if req.Position != nil {
		agent.Position = *req.Position
	}
	if req.Config != nil {
		agent.Config, _ = json.Marshal(req.Config)
	}

	if err := h.DB.Create(&agent).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.Hub.BroadcastEvent(wsID, "agents", "created", agent)

	return c.JSON(http.StatusCreated, agent)
}

// Update an existing agent
func (h *AgentHandler) Update(c echo.Context) error {
	agentID := c.Param("id")
	id, err := uuid.Parse(agentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
	}

	var agent models.Agent
	if err := h.DB.First(&agent, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var req UpdateAgentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if req.Name != nil {
		agent.Name = *req.Name
	}
	if req.ProviderType != nil {
		agent.ProviderType = *req.ProviderType
	}
	if req.Enabled != nil {
		agent.Enabled = *req.Enabled
	}
	if req.Position != nil {
		agent.Position = *req.Position
	}
	if req.Config != nil {
		agent.Config, _ = json.Marshal(req.Config)
	}

	if err := h.DB.Save(&agent).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.Hub.BroadcastEvent(agent.WorkspaceID, "agents", "updated", agent)

	return c.JSON(http.StatusOK, agent)
}

// Delete an agent
func (h *AgentHandler) Delete(c echo.Context) error {
	agentID := c.Param("id")
	id, err := uuid.Parse(agentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
	}

	var agent models.Agent
	if err := h.DB.First(&agent, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNoContent, nil) // Already deleted
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load agent for deletion (check encryption config): " + err.Error()})
	}
	wsID := agent.WorkspaceID

	if err := h.DB.Delete(&models.Agent{}, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.Hub.BroadcastEvent(wsID, "agents", "deleted", map[string]string{"id": agentID})

	return c.NoContent(http.StatusNoContent)
}

// Reorder agents
func (h *AgentHandler) Reorder(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var req ReorderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	tx := h.DB.Begin()
	for i, agentIDStr := range req.AgentIDs {
		agentID, err := uuid.Parse(agentIDStr)
		if err != nil {
			tx.Rollback()
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent_id in list"})
		}
		if err := tx.Model(&models.Agent{}).Where("id = ? AND workspace_id = ?", agentID, wsID).Update("position", i).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}
	tx.Commit()

	return c.JSON(http.StatusOK, map[string]string{"status": "reordered"})
}

// SpyPayload returns a redacted preview of what would be sent to the agent
func (h *AgentHandler) SpyPayload(c echo.Context) error {
	agentID := c.Param("id")
	id, err := uuid.Parse(agentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
	}

	question := c.QueryParam("question")
	if question == "" {
		question = "[Sample question for preview]"
	}

	var agent models.Agent
	if err := h.DB.First(&agent, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Redact sensitive fields
	config := make(map[string]any)
	json.Unmarshal(agent.Config, &config)

	sensitiveKeys := []string{"token", "api_key", "secret", "password", "key"}
	redactedConfig := make(map[string]any)
	for k, v := range config {
		isSensitive := false
		lowerKey := strings.ToLower(k)
		for _, sk := range sensitiveKeys {
			if strings.Contains(lowerKey, sk) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			if val, ok := v.(string); ok && len(val) > 4 {
				redactedConfig[k] = val[:2] + "****" + val[len(val)-2:]
			} else {
				redactedConfig[k] = "****"
			}
		} else {
			redactedConfig[k] = v
		}
	}

	payload := map[string]any{
		"request_id":    "[will be generated]",
		"provider_type": agent.ProviderType,
		"config":        redactedConfig,
		"payload": map[string]any{
			"question": question,
		},
	}

	return c.JSON(http.StatusOK, payload)
}
