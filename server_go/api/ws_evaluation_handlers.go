package api

import (
	"encoding/json"
	"strings"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (h *Hub) handleCreateEvaluation(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req models.CreateEvaluationPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	resultID, err := uuid.Parse(req.RunResultID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_result_id")
		return
	}

	// Logic for RatingCode and Rating mapping
	var ratingCode int
	var rating string

	switch {
	case req.RatingCode != nil:
		ratingCode = *req.RatingCode
		// Map back to rating string for compatibility
		switch ratingCode {
		case 1:
			rating = "like"
		case 2:
			rating = "valid"
		case 3:
			rating = "dislike"
		case 4:
			rating = "wrong"
		default:
			c.SendError(env.CorrelationID, "invalid rating_code")
			return
		}
	case req.Rating != "":
		rating = req.Rating
		switch rating {
		case "like":
			ratingCode = 1
		case "valid":
			ratingCode = 2
		case "dislike":
			ratingCode = 3
		case "wrong":
			ratingCode = 4
		default:
			c.SendError(env.CorrelationID, "invalid rating")
			return
		}
	default:
		c.SendError(env.CorrelationID, "rating or rating_code is required")
		return
	}

	score := req.Score
	if score == nil {
		defaultScore := 0
		switch ratingCode {
		case 1:
			defaultScore = 100
		case 2:
			defaultScore = 75
		case 3:
			defaultScore = 25
		case 4:
			defaultScore = 0
		}
		score = &defaultScore
	}

	var runResult models.RunResult
	if err := h.db.Select("id", "run_id").First(&runResult, "id = ?", resultID).Error; err != nil {
		c.SendError(env.CorrelationID, "run result not found")
		return
	}

	var run models.Run
	if err := h.db.Select("id", "question_set_id", "workspace_id").First(&run, "id = ?", runResult.RunID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	access, _, _, accessErr := h.getQuestionSetAccess(h.db, c.UserID, run.QuestionSetID)
	if accessErr != nil || !canReadQuestionSet(access) {
		c.SendError(env.CorrelationID, "access denied")
		return
	}

	eval := models.Evaluation{
		ID:          uuid.New(),
		RunResultID: resultID,
		RaterType:   "user",
		RaterID:     c.UserID,
		Rating:      rating,
		RatingCode:  &ratingCode,
		Score:       score,
		Comments:    req.Comments,
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_result_id = ? AND rater_type = ? AND rater_id = ?", resultID, "user", c.UserID).
			Delete(&models.Evaluation{}).Error; err != nil {
			return err
		}
		return tx.Create(&eval).Error
	}); err != nil {
		c.SendError(env.CorrelationID, "failed to create evaluation: "+err.Error())
		return
	}

	// Broadcast to the full question-set audience (owner + active
	// collaborators) so scores appear in real time on every connected side.
	// Fallback to workspace scope if the run can't be resolved (defensive —
	// shouldn't happen for a valid run_result_id).
	dataChanged := models.DataChangedPayload{
		Resource: "run_results",
		Action:   "updated",
		Data:     eval,
	}
	if sendErr := h.SendEventForRun(run.ID, EvtDataChanged, "", dataChanged); sendErr != nil {
		logger.Warn("[WS] handleCreateEvaluation: SendEventForRun failed, falling back to workspace broadcast: %v", sendErr)
		h.BroadcastEvent(c.WorkspaceID, EvtDataChanged, "", dataChanged)
	}

	c.SendResponse(DataEvaluation, env.CorrelationID, eval)
}

func (h *Hub) handleRunEvaluators(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req models.RunEvaluatorsPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	runID, err := uuid.Parse(req.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	var run models.Run
	if err := h.db.Select("id", "question_set_id").First(&run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}
	if access, _, _, accessErr := h.getQuestionSetAccess(h.db, c.UserID, run.QuestionSetID); accessErr != nil || !canWriteQuestionSet(access) {
		c.SendError(env.CorrelationID, "access denied")
		return
	}

	var selectedEvaluatorIDs []uuid.UUID
	seenEvaluatorIDs := make(map[uuid.UUID]struct{}, len(req.EvaluatorAgentIDs))
	for _, idStr := range req.EvaluatorAgentIDs {
		id, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			c.SendError(env.CorrelationID, "invalid evaluator_agent_id")
			return
		}
		if _, seen := seenEvaluatorIDs[id]; !seen {
			seenEvaluatorIDs[id] = struct{}{}
			selectedEvaluatorIDs = append(selectedEvaluatorIDs, id)
		}
	}
	if len(selectedEvaluatorIDs) == 0 {
		c.SendError(env.CorrelationID, "failed to run evaluators: no evaluator agents selected")
		return
	}

	for _, evaluatorID := range selectedEvaluatorIDs {
		if access, _, _, accessErr := h.getAgentAccess(h.db, c.UserID, evaluatorID); accessErr != nil {
			c.SendError(env.CorrelationID, "failed to verify evaluator access")
			return
		} else if access == agentAccessNone {
			c.SendError(env.CorrelationID, "evaluator agent is not accessible to you")
			return
		}
	}

	if err := h.engine.RunEvaluators(runID, selectedEvaluatorIDs); err != nil {
		c.SendError(env.CorrelationID, "failed to run evaluators: "+err.Error())
		return
	}

	if err := h.cacheAndSendResponse(c, DataResponse, env.CorrelationID, map[string]string{"status": "evaluators queued"}); err != nil {
		logger.Warn("[WS] handleRunEvaluators: send failed: %v", err)
	}
}

func (h *Hub) handleGetSpyPayload(c *Connection, env models.Envelope) {
	var req models.GetSpyPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	id, err := uuid.Parse(req.AgentID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent_id")
		return
	}

	question := req.Question
	if question == "" {
		question = "[Sample question for preview]"
	}

	// Only the owner and active collaborators may peek at the spy payload
	// (even though it's redacted, it still exposes provider_type, prompt
	// layout, and non-sensitive config knobs that are reasonable to gate).
	access, agent, _, accessErr := h.getAgentAccess(h.db, c.UserID, id)
	if accessErr != nil {
		c.SendError(env.CorrelationID, "agent not found")
		return
	}
	if access == agentAccessNone {
		c.SendError(env.CorrelationID, "agent is not accessible to you")
		return
	}

	// Redact sensitive fields (same logic as REST). We still redact for the
	// owner — the spy payload is a preview tool, not a credential export.
	config := make(map[string]any)
	if err := json.Unmarshal(agent.Config, &config); err != nil {
		logger.Warn("[EVAL] Failed to parse agent %s config for spy payload: %v", agent.ID, err)
	}

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

	c.SendResponse(DataSpyPayload, env.CorrelationID, payload)
}
