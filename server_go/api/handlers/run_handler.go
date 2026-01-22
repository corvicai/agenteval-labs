package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"benchmarking-platform/api"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"
)

type RunHandler struct {
	DB     *gorm.DB
	Engine *orchestrator.Engine
	Hub    api.HubInterface
}

func NewRunHandler(db *gorm.DB, engine *orchestrator.Engine, hub api.HubInterface) *RunHandler {
	return &RunHandler{DB: db, Engine: engine, Hub: hub}
}

// StartRunRequest represents the request to start a new run
type StartRunRequest struct {
	QuestionSetID string   `json:"question_set_id" validate:"required"`
	AgentIDs      []string `json:"agent_ids"` // Optional, if empty run all enabled agents
}

// RerunTaskRequest represents the request to rerun a single question
type RerunTaskRequest struct {
	AgentID    string `json:"agent_id" validate:"required"`
	QuestionID string `json:"question_id" validate:"required"`
}

// StartRun starts a new benchmark run
func (h *RunHandler) StartRun(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var req StartRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	qsID, err := uuid.Parse(req.QuestionSetID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid question_set_id"})
	}

	// Get question set
	var questionSet models.QuestionSet
	if err := h.DB.First(&questionSet, "id = ?", qsID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
	}

	// Get agents
	var agents []models.Agent
	if len(req.AgentIDs) > 0 {
		var agentUUIDs []uuid.UUID
		for _, idStr := range req.AgentIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			}
			agentUUIDs = append(agentUUIDs, id)
		}
		h.DB.Where("id IN ? AND workspace_id = ? AND enabled = true", agentUUIDs, wsID).Find(&agents)
	} else {
		// Get all enabled primary agents (not evaluators)
		h.DB.Where("workspace_id = ? AND enabled = true AND provider_type != 'evaluator'", wsID).Order("position ASC").Find(&agents)
	}

	if len(agents) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no agents available"})
	}

	unconfigured := findUnconfiguredAgents(agents)
	if len(unconfigured) > 0 {
		label := "agent not configured"
		if len(unconfigured) > 1 {
			label = "agents not configured"
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("%s: %s", label, strings.Join(unconfigured, ", "))})
	}

	// Parse questions from question set
	var qsData struct {
		Categories []struct {
			Name      string `json:"name"`
			Questions []struct {
				ID       any    `json:"id"`
				Question string `json:"question"`
				Expected string `json:"expected"`
			} `json:"questions"`
		} `json:"categories"`
	}
	json.Unmarshal(questionSet.Data, &qsData)

	// Calculate Total Tasks
	totalTasks := 0
	for _, cat := range qsData.Categories {
		totalTasks += len(cat.Questions)
	}
	totalTasks = totalTasks * len(agents)

	// Create run record
	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		QuestionSetID: qsID,
		Status:        "running",
		TotalTasks:    totalTasks,
	}
	if err := h.DB.Create(&run).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Queue tasks for each agent + question
	for _, agent := range agents {
		var agentConfig map[string]any
		json.Unmarshal(agent.Config, &agentConfig)

		for _, cat := range qsData.Categories {
			for _, q := range cat.Questions {
				qID := ""
				switch v := q.ID.(type) {
				case string:
					qID = v
				case float64:
					qID = fmt.Sprintf("%.0f", v)
				default:
					qID = "unknown"
				}

				task := &orchestrator.Task{
					RunID:        run.ID,
					AgentID:      agent.ID,
					QuestionID:   qID,
					QuestionText: q.Question,
					AgentConfig:  agentConfig,
					ProviderType: agent.ProviderType,
				}
				h.Engine.QueueTask(task)
			}
		}
	}

	h.Hub.BroadcastEvent(wsID, "runs", "created", run)

	return c.JSON(http.StatusAccepted, map[string]any{
		"run_id":      run.ID,
		"status":      "running",
		"agent_count": len(agents),
		"total_tasks": totalTasks,
	})
}

func findUnconfiguredAgents(agents []models.Agent) []string {
	var unconfigured []string
	for _, agent := range agents {
		if !isAgentConfigured(agent) {
			unconfigured = append(unconfigured, agent.Name)
		}
	}
	return unconfigured
}

func isAgentConfigured(agent models.Agent) bool {
	if len(agent.Config) == 0 {
		return false
	}

	var config map[string]any
	if err := json.Unmarshal(agent.Config, &config); err != nil {
		return false
	}
	if len(config) == 0 {
		return false
	}

	switch agent.ProviderType {
	case "mcp":
		endpoint := strings.TrimSpace(getConfigString(config, "endpoint"))
		token := strings.TrimSpace(getConfigString(config, "token"))
		if endpoint == "" || token == "" {
			return false
		}
	case "openai", "evaluator":
		apiKey := strings.TrimSpace(getConfigString(config, "api_key"))
		if apiKey == "" {
			return false
		}
	}

	return true
}

func getConfigString(config map[string]any, key string) string {
	if val, ok := config[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// RerunTask reruns a single question for a specific agent
func (h *RunHandler) RerunTask(c echo.Context) error {
	runID := c.Param("run_id")
	rID, err := uuid.Parse(runID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
	}

	var req RerunTaskRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
	}

	// Get existing run
	var run models.Run
	if err := h.DB.First(&run, "id = ?", rID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	// Get agent
	var agent models.Agent
	if err := h.DB.First(&agent, "id = ?", agentID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}

	// Get question set to find the question text
	var questionSet models.QuestionSet
	if err := h.DB.First(&questionSet, "id = ?", run.QuestionSetID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set not found"})
	}

	var qsData struct {
		Categories []struct {
			Questions []struct {
				ID       any    `json:"id"`
				Question string `json:"question"`
			} `json:"questions"`
		} `json:"categories"`
	}
	json.Unmarshal(questionSet.Data, &qsData)

	// Find the question text
	var questionText string
	for _, cat := range qsData.Categories {
		for _, q := range cat.Questions {
			qID := ""
			switch v := q.ID.(type) {
			case string:
				qID = v
			case float64:
				qID = fmt.Sprintf("%.0f", v)
			}
			if qID == req.QuestionID {
				questionText = q.Question
				break
			}
		}
	}

	if questionText == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question not found"})
	}

	var agentConfig map[string]any
	json.Unmarshal(agent.Config, &agentConfig)

	task := &orchestrator.Task{
		RunID:        run.ID,
		AgentID:      agent.ID,
		QuestionID:   req.QuestionID,
		QuestionText: questionText,
		AgentConfig:  agentConfig,
		ProviderType: agent.ProviderType,
	}
	h.Engine.QueueTask(task)

	return c.JSON(http.StatusAccepted, map[string]string{"status": "queued"})
}

// GetRunStatus returns the status of a run
func (h *RunHandler) GetRunStatus(c echo.Context) error {
	runID := c.Param("run_id")
	rID, err := uuid.Parse(runID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
	}

	var run models.Run
	if err := h.DB.Preload("Results").First(&run, "id = ?", rID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, run)
}

// GetHistory returns the answer history for a specific question and agent
func (h *RunHandler) GetHistory(c echo.Context) error {
	agentID := c.QueryParam("agent_id")
	questionID := c.QueryParam("question_id")
	workspaceID := c.Param("workspace_id")

	aID, err := uuid.Parse(agentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
	}

	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	var results []models.RunResult
	query := h.DB.
		Joins("JOIN runs ON runs.id = run_results.run_id").
		Where("run_results.agent_id = ? AND runs.workspace_id = ?", aID, wsID)

	if questionID != "" {
		query = query.Where("run_results.question_id = ?", questionID)
	}

	if err := query.Preload("Evaluations").Order("run_results.created_at DESC").Limit(50).Find(&results).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, results)
}

// GetWorkspaceRuns returns all runs for a specific workspace
func (h *RunHandler) GetWorkspaceRuns(c echo.Context) error {
	workspaceID := c.Param("workspace_id")
	wsID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid workspace_id"})
	}

	// Pagination params
	limit := 20
	offset := 0

	var runs []models.Run
	// Preload minimal info if needed, but for list just run info is enough
	if err := h.DB.Where("workspace_id = ?", wsID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&runs).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// We'll fetch QuestionSet names manually in the loop below

	// Create response with Run + QuestionSetName + Count of Results
	type RunResponse struct {
		models.Run
		QuestionSetName string `json:"question_set_name"`
		ResultCount     int    `json:"result_count"`
	}

	var response []RunResponse
	for _, run := range runs {
		// Count results
		var count int64
		h.DB.Model(&models.RunResult{}).Where("run_id = ?", run.ID).Count(&count)

		var qsName string
		// Fetch QS name manually if Preload fails or just to be safe (Preload works if relationship defined)
		// Run struct doesn't have QuestionSet relation field defined in models.go shown above?
		// Let's check models.go again. Run struct has QuestionSetID but no *QuestionSet field.
		// So Preload("QuestionSet") usage above relies on association being there.
		// Looking at models.go: Yes, "QuestionSetID" is there but no "QuestionSet" field.
		// I must fix this logic. I cannot use Preload("QuestionSet") if the field isn't there.
		// I will do a manual lookup for now or just return generic list.

		var qs models.QuestionSet
		h.DB.First(&qs, "id = ?", run.QuestionSetID)
		qsName = qs.Name

		response = append(response, RunResponse{
			Run:             run,
			QuestionSetName: qsName,
			ResultCount:     int(count),
		})
	}

	return c.JSON(http.StatusOK, response)
}

// GetRun returns full details of a run including results
func (h *RunHandler) GetRun(c echo.Context) error {
	runID := c.Param("run_id")
	rID, err := uuid.Parse(runID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
	}

	var run models.Run
	if err := h.DB.First(&run, "id = ?", rID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	// Get QuestionSet Data for context
	var qs models.QuestionSet
	if err := h.DB.First(&qs, "id = ?", run.QuestionSetID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "question set associated with run not found"})
	}

	// Get Results with Agents
	var results []models.RunResult
	if err := h.DB.Preload("Evaluations").Where("run_id = ?", rID).Order("created_at ASC").Find(&results).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Collect Agent info for results
	agentMap := make(map[uuid.UUID]models.Agent)
	for _, res := range results {
		if _, exists := agentMap[res.AgentID]; !exists {
			var agent models.Agent
			if err := h.DB.First(&agent, "id = ?", res.AgentID).Error; err == nil {
				agentMap[res.AgentID] = agent
			}
		}
	}

	// Construct response
	return c.JSON(http.StatusOK, map[string]any{
		"run":          run,
		"question_set": qs,
		"results":      results,
		"agents":       agentMap,
	})
}
