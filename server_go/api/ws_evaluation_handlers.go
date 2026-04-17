package api

import (
	"encoding/json"
	"strings"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

func (h *Hub) handleCreateEvaluation(c *Connection, env models.Envelope) {
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

	if req.RatingCode != nil {
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
	} else if req.Rating != "" {
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
	} else {
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

	if err := h.db.Create(&eval).Error; err != nil {
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
	var runRow struct {
		RunID uuid.UUID `gorm:"column:run_id"`
	}
	if err := h.db.Raw(`SELECT run_id FROM run_results WHERE id = ? LIMIT 1`, resultID).Scan(&runRow).Error; err != nil || runRow.RunID == uuid.Nil {
		logger.Warn("[WS] handleCreateEvaluation: could not resolve run for result %s: %v", resultID, err)
		h.BroadcastEvent(c.WorkspaceID, EvtDataChanged, "", dataChanged)
	} else if sendErr := h.SendEventForRun(runRow.RunID, EvtDataChanged, "", dataChanged); sendErr != nil {
		logger.Warn("[WS] handleCreateEvaluation: SendEventForRun failed, falling back to workspace broadcast: %v", sendErr)
		h.BroadcastEvent(c.WorkspaceID, EvtDataChanged, "", dataChanged)
	}

	c.SendResponse(DataEvaluation, env.CorrelationID, eval)
}

func (h *Hub) handleRunEvaluators(c *Connection, env models.Envelope) {
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

	var selectedEvaluatorIDs []uuid.UUID
	for _, idStr := range req.EvaluatorAgentIDs {
		if id, parseErr := uuid.Parse(idStr); parseErr == nil {
			selectedEvaluatorIDs = append(selectedEvaluatorIDs, id)
		}
	}
	if len(selectedEvaluatorIDs) == 0 {
		c.SendError(env.CorrelationID, "failed to run evaluators: no evaluator agents selected")
		return
	}

	if err := h.engine.RunEvaluators(runID, selectedEvaluatorIDs); err != nil {
		c.SendError(env.CorrelationID, "failed to run evaluators: "+err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "evaluators queued"})
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
