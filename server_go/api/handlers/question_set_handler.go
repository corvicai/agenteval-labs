package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"benchmarking-platform/api"
	"benchmarking-platform/models"
)

type QuestionSetHandler struct {
	DB  *gorm.DB
	Hub api.HubInterface
}

func NewQuestionSetHandler(db *gorm.DB, hub api.HubInterface) *QuestionSetHandler {
	return &QuestionSetHandler{DB: db, Hub: hub}
}

// ListClients lists all clients for a workspace
func (h *QuestionSetHandler) ListClients(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var clients []models.Client
	if err := h.DB.Where("workspace_id = ?", wsID).Find(&clients).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, clients)
}

// QuestionData represents the import/export format
type QuestionData struct {
	Categories []Category `json:"categories"`
}

type Category struct {
	Name      string     `json:"name"`
	Questions []Question `json:"questions"`
}

type Question struct {
	ID       any    `json:"id,omitempty"`
	Question string `json:"question"`
	Expected string `json:"expected,omitempty"`
}

// CreateQuestionSetRequest represents the request body
type CreateQuestionSetRequest struct {
	Name    string       `json:"name" validate:"required"`
	Version string       `json:"version"`
	Data    QuestionData `json:"data" validate:"required"`
}

// List all question sets for a client
func (h *QuestionSetHandler) List(c echo.Context) error {
	clientID := c.Param("client_id")
	cID, err := uuid.Parse(clientID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid client_id"})
	}

	var sets []models.QuestionSet
	if err := h.DB.Where("client_id = ?", cID).Find(&sets).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, sets)
}

// Get a single question set
func (h *QuestionSetHandler) Get(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var set models.QuestionSet
	if err := h.DB.First(&set, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, set)
}

// Import creates a new question set from the standard format
func (h *QuestionSetHandler) Import(c echo.Context) error {
	clientID := c.Param("client_id")
	cID, err := uuid.Parse(clientID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid client_id"})
	}

	var req CreateQuestionSetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Assign stable IDs to questions that don't have them
	for catIdx, cat := range req.Data.Categories {
		for qIdx := range cat.Questions {
			if req.Data.Categories[catIdx].Questions[qIdx].ID == nil {
				// Use stable integer-based IDs: category_idx * 1000 + question_idx
				req.Data.Categories[catIdx].Questions[qIdx].ID = catIdx*1000 + qIdx
			}
		}
	}

	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal data"})
	}

	set := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: cID,
		Name:     req.Name,
		Version:  req.Version,
		Data:     datatypes.JSON(dataBytes),
	}

	if err := h.DB.Create(&set).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// We need the workspace ID from the client to broadcast
	var client models.Client
	h.DB.First(&client, "id = ?", cID)
	h.Hub.BroadcastEvent(client.WorkspaceID, "question_sets", "created", set)

	return c.JSON(http.StatusCreated, set)
}

// Export returns the question set in the standard format
func (h *QuestionSetHandler) Export(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var set models.QuestionSet
	if err := h.DB.First(&set, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var data QuestionData
	if err := json.Unmarshal(set.Data, &data); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse data"})
	}

	return c.JSON(http.StatusOK, data)
}

// Delete a question set
func (h *QuestionSetHandler) Delete(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var set models.QuestionSet
	h.DB.Preload("Client").First(&set, "id = ?", id)
	wsID := set.Client.WorkspaceID

	if err := h.DB.Delete(&models.QuestionSet{}, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.Hub.BroadcastEvent(wsID, "question_sets", "deleted", map[string]string{"id": setID})

	return c.NoContent(http.StatusNoContent)
}

// Update updates an existing question set
func (h *QuestionSetHandler) Update(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var req CreateQuestionSetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var set models.QuestionSet
	if err := h.DB.First(&set, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
	}

	// Assign stable IDs to questions that don't have them
	for catIdx, cat := range req.Data.Categories {
		for qIdx := range cat.Questions {
			if req.Data.Categories[catIdx].Questions[qIdx].ID == nil {
				req.Data.Categories[catIdx].Questions[qIdx].ID = catIdx*1000 + qIdx
			}
		}
	}

	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal data"})
	}

	set.Name = req.Name
	set.Version = req.Version
	set.Data = datatypes.JSON(dataBytes)

	if err := h.DB.Save(&set).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Fetch workspace ID via client if not preloaded
	h.DB.Preload("Client").First(&set, "id = ?", id)
	h.Hub.BroadcastEvent(set.Client.WorkspaceID, "question_sets", "updated", set)

	return c.JSON(http.StatusOK, set)
}

// GetAgents returns the agents configured for a specific question set
// If no specific configuration exists, returns all workspace agents (backward compatibility)
func (h *QuestionSetHandler) GetAgents(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	// Get the question set to find the workspace
	var qs models.QuestionSet
	if err := h.DB.Preload("Client").First(&qs, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
	}

	// Get workspace from client
	var client models.Client
	if err := h.DB.First(&client, "id = ?", qs.ClientID).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "client not found"})
	}

	// Check if this QS has specific agent configuration
	var qsAgents []models.QuestionSetAgent
	h.DB.Preload("Agent").Where("question_set_id = ?", id).Order("position").Find(&qsAgents)

	if len(qsAgents) > 0 {
		// Return QS-specific agents
		agents := make([]models.Agent, 0)
		for _, qsa := range qsAgents {
			agent := qsa.Agent
			agent.Enabled = qsa.Enabled
			agent.Position = qsa.Position
			// Override config if present in junction table
			if qsa.Config != nil {
				agent.Config = qsa.Config
			}
			agents = append(agents, agent)
		}
		return c.JSON(http.StatusOK, agents)
	}

	// Fallback: return all workspace agents
	var agents []models.Agent
	h.DB.Where("workspace_id = ?", client.WorkspaceID).Order("position").Find(&agents)
	return c.JSON(http.StatusOK, agents)
}

// UpdateAgentsRequest represents the request body for updating QS agents
type UpdateAgentsRequest struct {
	Agents []AgentConfig `json:"agents"`
}

type AgentConfig struct {
	AgentID  string               `json:"agent_id"`
	Enabled  bool                 `json:"enabled"`
	Position int                  `json:"position"`
	Config   models.EncryptedJSON `json:"config"`
}

// UpdateAgents updates the agent configuration for a specific question set
func (h *QuestionSetHandler) UpdateAgents(c echo.Context) error {
	setID := c.Param("id")
	id, err := uuid.Parse(setID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var req UpdateAgentsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Verify QS exists
	var qs models.QuestionSet
	if err := h.DB.First(&qs, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
	}

	// Delete existing QS agent configurations
	h.DB.Where("question_set_id = ?", id).Delete(&models.QuestionSetAgent{})

	// Insert new configurations
	for _, ac := range req.Agents {
		agentID, err := uuid.Parse(ac.AgentID)
		if err != nil {
			continue
		}
		qsAgent := models.QuestionSetAgent{
			QuestionSetID: id,
			AgentID:       agentID,
			Enabled:       ac.Enabled,
			Position:      ac.Position,
			Config:        ac.Config,
		}
		h.DB.Select("QuestionSetID", "AgentID", "Enabled", "Position", "Config").Create(&qsAgent)
	}

	// Return updated agents
	return h.GetAgents(c)
}
