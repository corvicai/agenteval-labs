package api

import (
	"encoding/json"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

// handlePing responds to REQ_PING with a DATA_PONG carrying the server timestamp.
// Used by the client heartbeat (Plan 25B) to detect zombie connections.
func (h *Hub) handlePing(c *Connection, env models.Envelope) {
	c.SendResponse(DataPong, env.CorrelationID, map[string]any{
		"ts": time.Now().UTC().UnixMilli(),
	})
}

// handleGetPendingResponse serves REQ_GET_PENDING_RESPONSE.
//
// After a transient disconnect the client may have in-flight requests whose
// responses were sent while the connection was down. This handler looks up the
// correlation_id in the short-lived response cache and re-delivers the original
// payload if still available (Plan 24, Layer 4).
//
// Payload: { correlation_id: string }
// Response DATA_PENDING_RESPONSE:
//
//	{ found: false }                                  — not cached / expired
//	{ found: true, msg_type: string, payload: any }   — re-delivered
func (h *Hub) handleGetPendingResponse(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req struct {
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.Unmarshal(env.Payload, &req); err != nil || req.CorrelationID == "" {
		c.SendError(env.CorrelationID, "invalid payload: correlation_id required")
		return
	}

	cached, found := h.responseCache.Get(req.CorrelationID)
	if !found {
		if err := c.SendResponse(DataPendingResponse, env.CorrelationID, map[string]any{
			"found": false,
		}); err != nil {
			logger.Warn("[WS] handleGetPendingResponse: send failed: %v", err)
		}
		return
	}

	if err := c.SendResponse(DataPendingResponse, env.CorrelationID, map[string]any{
		"found":    true,
		"msg_type": cached.MsgType,
		"payload":  json.RawMessage(cached.PayloadJSON),
	}); err != nil {
		logger.Warn("[WS] handleGetPendingResponse: send failed: %v", err)
	}
}

// handleGetRunProgress serves REQ_GET_RUN_PROGRESS.
//
// Payload: { run_id: string, since_ts?: string }   (since_ts is RFC3339 or empty)
// Response DATA_RUN_PROGRESS: { run_id, status, total_tasks, results_since, result_count }
//
// If since_ts is provided only results created after that time are returned.
// This lets the frontend reconcile results it may have missed during a disconnect
// without fetching the full run details payload (Plan 24, Layer 3).
func (h *Hub) handleGetRunProgress(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var payload struct {
		RunID   string `json:"run_id"`
		SinceTs string `json:"since_ts"`
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

	// Verify the run belongs to the caller's workspace
	var run models.Run
	if err := h.db.First(&run, "id = ? AND workspace_id = ?", runID, c.WorkspaceID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Parse optional since_ts filter
	var sinceTime *time.Time
	if payload.SinceTs != "" {
		if t, err := time.Parse(time.RFC3339Nano, payload.SinceTs); err == nil {
			sinceTime = &t
		} else if t, err := time.Parse(time.RFC3339, payload.SinceTs); err == nil {
			sinceTime = &t
		}
	}

	// Query run results (delta only when since_ts is provided)
	var results []models.RunResult
	q := h.db.Where("run_id = ?", runID)
	if sinceTime != nil {
		q = q.Where("created_at > ?", *sinceTime)
	}
	q.Find(&results)
	normalizeResultsEvaluationsForDisplay(results)

	c.SendResponse(DataRunProgress, env.CorrelationID, map[string]any{
		"run_id":        run.ID,
		"status":        run.Status,
		"total_tasks":   run.TotalTasks,
		"result_count":  len(results),
		"results_since": results,
	})
}
