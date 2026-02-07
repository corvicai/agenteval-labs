package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (h *Hub) handleStartRun(c *Connection, env models.Envelope) {
	var payload models.StartRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	var agentIDs []uuid.UUID
	for _, idStr := range payload.AgentIDs {
		if id, err := uuid.Parse(idStr); err == nil {
			agentIDs = append(agentIDs, id)
		}
	}

	// Validate Agent Credentials
	if len(agentIDs) > 0 {
		var agents []models.Agent
		if err := h.db.Find(&agents, agentIDs).Error; err != nil {
			c.SendError(env.CorrelationID, "failed to load agents for validation")
			return
		}

		for _, agent := range agents {
			// Skip disabled agents? UI sends selected agents. We validate all selected.
			var config map[string]interface{}
			if err := json.Unmarshal(agent.Config, &config); err != nil {
				continue
			}

			switch agent.ProviderType {
			case "openai":
				if v, ok := config["api_key"].(string); !ok || v == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (API Key is required)", agent.Name))
					return
				}
			case "evaluator":
				if v, ok := config["api_key"].(string); !ok || v == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (API Key is required)", agent.Name))
					return
				}
			case "mcp":
				mode, _ := config["mode"].(string)
				if mode == "http" || mode == "" {
					endpoint, _ := config["endpoint"].(string)
					token, _ := config["token"].(string)
					// User requested check for BOTH endpoint and token
					if endpoint == "" || token == "" {
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (Endpoint and Token are required)", agent.Name))
						return
					}
				}
			}
		}
	}

	// For legacy support, we use the connection's workspace
	run, err := h.engine.StartRun(c.WorkspaceID, qsID, agentIDs)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, run)
}

func (h *Hub) handleRerunTask(c *Connection, env models.Envelope) {
	var payload models.RerunTaskPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, _ := uuid.Parse(payload.RunID)
	agentID, _ := uuid.Parse(payload.AgentID)

	// Pass frontend-provided context to engine
	opts := &orchestrator.RerunTaskOptions{
		OriginalQuestion: payload.OriginalQuestion,
		ExpectedAnswer:   payload.ExpectedAnswer,
		QuestionSetID:    payload.QuestionSetID,
		ResultID:         payload.ResultID,
	}

	if err := h.engine.RerunTask(runID, agentID, payload.QuestionID, opts); err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "queued"})
}

func (h *Hub) handleCancelRun(c *Connection, env models.Envelope) {
	var payload models.CancelRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, _ := uuid.Parse(payload.RunID)
	h.engine.CancelRun(runID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "cancelled"})
}

func (h *Hub) handleSyncState(c *Connection, env models.Envelope) {
	var payload models.SyncStatePayload

	log.Printf("[WS] SyncState requested for workspace: %s", c.WorkspaceID)

	// 1. Get Agents
	if err := h.db.Where("workspace_id = ?", c.WorkspaceID).Order("created_at desc").Find(&payload.Agents).Error; err != nil {
		log.Printf("[WS] SyncState error loading agents: %v", err)
		c.SendError(env.CorrelationID, "failed to load agents: "+err.Error())
		return
	}

	// 2. Get Question Sets
	if err := h.db.Model(&models.QuestionSet{}).
		Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("clients.workspace_id = ?", c.WorkspaceID).
		Preload("Client").
		Preload("Agents").
		Order("question_sets.created_at desc").
		Find(&payload.QuestionSets).Error; err != nil {
		log.Printf("[WS] SyncState error loading question sets: %v", err)
		c.SendError(env.CorrelationID, "failed to load question sets: "+err.Error())
		return
	}

	// 3. Get Recent Runs (last 10)
	if err := h.db.Raw(`
		SELECT r.*, qs.name as question_set_name
		FROM runs r
		JOIN question_sets qs ON r.question_set_id = qs.id
		WHERE r.workspace_id = ?
		ORDER BY r.created_at desc
		LIMIT 10
	`, c.WorkspaceID).Scan(&payload.RecentRuns).Error; err != nil {
		log.Printf("[WS] SyncState error loading recent runs: %v", err)
		c.SendError(env.CorrelationID, "failed to load recent runs: "+err.Error())
		return
	}

	log.Printf("[WS] SyncState completed for workspace: %s (Agents: %d, Sets: %d, Runs: %d)",
		c.WorkspaceID, len(payload.Agents), len(payload.QuestionSets), len(payload.RecentRuns))

	c.SendResponse(DataState, env.CorrelationID, payload)
}

func (h *Hub) handleGetRunDetails(c *Connection, env models.Envelope) {
	var req models.GetRunDetailsPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	runID, err := uuid.Parse(req.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	var response models.RunDetailsResponse

	// 1. Get Run
	if err := h.db.First(&response.Run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// 2. Get Question Set
	if err := h.db.Preload("Client").Preload("Agents").First(&response.QuestionSet, "id = ?", response.Run.QuestionSetID).Error; err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	// 3. Get Results (including evaluations)
	if err := h.db.Preload("Evaluations").Where("run_id = ?", runID).Find(&response.Results).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to load results: "+err.Error())
		return
	}

	// 4. Collect Agent info
	response.Agents = make(map[string]models.Agent)
	for _, res := range response.Results {
		if _, exists := response.Agents[res.AgentID.String()]; !exists {
			var agent models.Agent
			if err := h.db.First(&agent, "id = ?", res.AgentID).Error; err == nil {
				response.Agents[res.AgentID.String()] = agent
			}
		}
	}

	c.SendResponse(DataRunDetails, env.CorrelationID, response)
}

func (h *Hub) handleGetWorkspaceRuns(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	if c.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "no workspace selected")
		return
	}

	var runs []models.Run
	// Verify workspace belongs to the user to prevent unauthorized access
	var ws models.Workspace
	if err := h.db.First(&ws, "id = ? AND user_id = ?", c.WorkspaceID, c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "workspace not found or access denied")
		return
	}

	if err := h.db.Raw(`
		SELECT r.*, qs.name as question_set_name
		FROM runs r
		JOIN question_sets qs ON r.question_set_id = qs.id
		WHERE r.workspace_id = ?
		ORDER BY r.created_at DESC
	`, c.WorkspaceID).Scan(&runs).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to load runs: "+err.Error())
		return
	}

	c.SendResponse(DataWorkspaceRuns, env.CorrelationID, runs)
}

func (h *Hub) handleGetRunLite(c *Connection, env models.Envelope) {
	var payload models.GetRunLitePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, err := uuid.Parse(payload.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	// Fetch Run
	var run models.Run
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Fetch QuestionSet
	var qs models.QuestionSet
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", run.QuestionSetID)

	// Fetch Results (Lite: Select specific columns only)
	// We need 'answer' to compute hash, but we won't send it.
	// GORM: defining a temporary struct for DB scanning is cleaner.
	type ResultScan struct {
		ID         uuid.UUID
		RunID      uuid.UUID
		AgentID    uuid.UUID
		QuestionID string
		Status     string
		DurationMs int
		CreatedAt  time.Time
		Answer     string
	}
	var scanned []ResultScan

	err = h.db.Model(&models.RunResult{}).
		Select("id, run_id, agent_id, question_id, status, duration_ms, created_at, answer").
		Where("run_id = ?", runID).
		Scan(&scanned).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch results")
		return
	}

	// Map Scan -> Lite and compute Hash
	results := make([]models.RunResultLite, len(scanned))
	for i, s := range scanned {
		hash := ""
		if s.Answer != "" {
			hObj := sha256.New()
			hObj.Write([]byte(s.Answer))
			hash = hex.EncodeToString(hObj.Sum(nil))
		}

		results[i] = models.RunResultLite{
			ID:          s.ID,
			RunID:       s.RunID,
			AgentID:     s.AgentID,
			QuestionID:  s.QuestionID,
			Status:      s.Status,
			ContentHash: hash,
			DurationMs:  s.DurationMs,
			CreatedAt:   s.CreatedAt,
		}
	}

	// Fetch Evaluations existence (to set HasEvaluations flag)
	// Optimize: Get all result IDs that have evaluations
	var resultIDsWithEvals []uuid.UUID
	h.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON evaluations.run_result_id = run_results.id").
		Where("run_results.run_id = ?", runID).
		Distinct("run_results.id").
		Pluck("run_results.id", &resultIDsWithEvals)

	// Create a map for quick lookup
	evalsMap := make(map[uuid.UUID]bool)
	for _, id := range resultIDsWithEvals {
		evalsMap[id] = true
	}

	// Update results with HasEvaluations flag
	for i := range results {
		if evalsMap[results[i].ID] {
			results[i].HasEvaluations = true
		}
	}

	// Fetch Agents (Snapshot or Current)
	// Ideally we should store agent snapshot in Run, but for now we look up current agents
	// or try to reconstruct. Here we just fetch all agents referenced in the run results.
	agentIDs := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]bool)
	for _, res := range results {
		if !seen[res.AgentID] {
			agentIDs = append(agentIDs, res.AgentID)
			seen[res.AgentID] = true
		}
	}

	agents := make(map[string]models.Agent)
	if len(agentIDs) > 0 {
		var agentList []models.Agent
		h.db.Find(&agentList, agentIDs)
		for _, a := range agentList {
			agents[a.ID.String()] = a
		}
	}

	resp := models.RunLiteResponse{
		Run:         run,
		QuestionSet: qs,
		Results:     results,
		Agents:      agents,
	}

	c.SendResponse(DataRunLite, env.CorrelationID, resp)
}

func (h *Hub) handleGetLatestRunByQuestionSet(c *Connection, env models.Envelope) {
	var payload models.GetLatestRunByQSPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	var run models.Run
	if err := h.db.Where("workspace_id = ? AND question_set_id = ? AND status != ?", c.WorkspaceID, qsID, "running").
		Order("created_at desc").
		First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.SendResponse(DataRunLite, env.CorrelationID, map[string]any{
				"run":          nil,
				"question_set": nil,
				"results":      []models.RunResultLite{},
				"agents":       map[string]models.Agent{},
			})
			return
		}
		c.SendError(env.CorrelationID, "failed to fetch run")
		return
	}

	var qs models.QuestionSet
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", run.QuestionSetID)

	type ResultScan struct {
		ID         uuid.UUID
		RunID      uuid.UUID
		AgentID    uuid.UUID
		QuestionID string
		Status     string
		DurationMs int
		CreatedAt  time.Time
		Answer     string
		Error      string
	}
	var scanned []ResultScan

	err = h.db.Model(&models.RunResult{}).
		Select("id, run_id, agent_id, question_id, status, duration_ms, created_at, answer, error").
		Where("run_id = ?", run.ID).
		Scan(&scanned).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch results")
		return
	}

	results := make([]models.RunResultLite, len(scanned))
	for i, s := range scanned {
		hash := ""
		if s.Answer != "" {
			hObj := sha256.New()
			hObj.Write([]byte(s.Answer))
			hash = hex.EncodeToString(hObj.Sum(nil))
		}

		results[i] = models.RunResultLite{
			ID:          s.ID,
			RunID:       s.RunID,
			AgentID:     s.AgentID,
			QuestionID:  s.QuestionID,
			Status:      s.Status,
			ContentHash: hash,
			Error:       s.Error,
			DurationMs:  s.DurationMs,
			CreatedAt:   s.CreatedAt,
		}
	}

	var resultIDsWithEvals []uuid.UUID
	h.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON evaluations.run_result_id = run_results.id").
		Where("run_results.run_id = ?", run.ID).
		Distinct("run_results.id").
		Pluck("run_results.id", &resultIDsWithEvals)

	evalsMap := make(map[uuid.UUID]bool)
	for _, id := range resultIDsWithEvals {
		evalsMap[id] = true
	}

	for i := range results {
		if evalsMap[results[i].ID] {
			results[i].HasEvaluations = true
		}
	}

	agentIDs := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]bool)
	for _, res := range results {
		if !seen[res.AgentID] {
			agentIDs = append(agentIDs, res.AgentID)
			seen[res.AgentID] = true
		}
	}

	agents := make(map[string]models.Agent)
	if len(agentIDs) > 0 {
		var agentList []models.Agent
		h.db.Find(&agentList, agentIDs)
		for _, a := range agentList {
			agents[a.ID.String()] = a
		}
	}

	resp := models.RunLiteResponse{
		Run:         run,
		QuestionSet: qs,
		Results:     results,
		Agents:      agents,
	}

	c.SendResponse(DataRunLite, env.CorrelationID, resp)
}

func (h *Hub) handleGetResultDetails(c *Connection, env models.Envelope) {
	var payload models.GetResultDetailsPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if len(payload.ResultIDs) == 0 {
		c.SendResponse(DataResultDetails, env.CorrelationID, models.ResultDetailsResponse{Results: []models.RunResult{}})
		return
	}

	var results []models.RunResult
	err := h.db.Preload("Evaluations").
		Where("id IN ?", payload.ResultIDs).
		Find(&results).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch details")
		return
	}

	c.SendResponse(DataResultDetails, env.CorrelationID, models.ResultDetailsResponse{Results: results})
}

func (h *Hub) handleDeleteRun(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, err := uuid.Parse(payload.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	var run models.Run
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Verify ownership
	if run.WorkspaceID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete Evaluations
		if err := tx.Exec(`
			DELETE FROM evaluations 
			WHERE run_result_id IN (SELECT id FROM run_results WHERE run_id = ?)
		`, runID).Error; err != nil {
			return err
		}

		// 2. Delete RunResults
		if err := tx.Where("run_id = ?", runID).Delete(&models.RunResult{}).Error; err != nil {
			return err
		}

		// 3. Delete Run
		if err := tx.Delete(&run).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete run", err.Error())
		return
	}

	h.BroadcastEvent(run.WorkspaceID, "runs", "deleted", map[string]string{"id": runID.String()})
	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}

func (h *Hub) handleDeleteAllRuns(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	if c.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "no workspace selected")
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete Evaluations for all runs in workspace
		if err := tx.Exec(`
			DELETE FROM evaluations 
			WHERE run_result_id IN (
				SELECT rr.id FROM run_results rr
				JOIN runs r ON rr.run_id = r.id
				WHERE r.workspace_id = ?
			)
		`, c.WorkspaceID).Error; err != nil {
			return err
		}

		// 2. Delete RunResults
		if err := tx.Exec(`
			DELETE FROM run_results 
			WHERE run_id IN (SELECT id FROM runs WHERE workspace_id = ?)
		`, c.WorkspaceID).Error; err != nil {
			return err
		}

		// 3. Delete Runs
		if err := tx.Where("workspace_id = ?", c.WorkspaceID).Delete(&models.Run{}).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete history", err.Error())
		return
	}

	h.BroadcastEvent(c.WorkspaceID, "runs", "all_deleted", nil)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}
