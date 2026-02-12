package orchestrator

import (
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
)

const retryStateTTL = 30 * time.Minute

type retryState struct {
	RetryID     string
	RunID       uuid.UUID
	WorkspaceID uuid.UUID
	AgentID     uuid.UUID
	QuestionID  string
	Status      string
	RunResultID uuid.UUID
	Error       string
	DurationMs  int
	UpdatedAt   time.Time
}

func (e *Engine) pruneRetryStatesLocked(now time.Time) {
	for retryID, state := range e.retryStates {
		if now.Sub(state.UpdatedAt) > retryStateTTL {
			delete(e.retryStates, retryID)
		}
	}
}

func (e *Engine) setRetryState(
	retryID string,
	runID uuid.UUID,
	workspaceID uuid.UUID,
	agentID uuid.UUID,
	questionID string,
	status string,
	runResultID uuid.UUID,
	errMsg string,
	durationMs int,
) {
	if retryID == "" {
		return
	}

	now := time.Now().UTC()
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pruneRetryStatesLocked(now)

	existing := e.retryStates[retryID]
	if existing.RetryID == "" {
		existing = retryState{
			RetryID: retryID,
		}
	}

	if runID != uuid.Nil {
		existing.RunID = runID
	}
	if workspaceID != uuid.Nil {
		existing.WorkspaceID = workspaceID
	}
	if agentID != uuid.Nil {
		existing.AgentID = agentID
	}
	if questionID != "" {
		existing.QuestionID = questionID
	}
	if status != "" {
		existing.Status = status
	}
	if runResultID != uuid.Nil {
		existing.RunResultID = runResultID
	}
	if errMsg != "" {
		existing.Error = errMsg
	}
	if durationMs > 0 {
		existing.DurationMs = durationMs
	}
	existing.UpdatedAt = now

	e.retryStates[retryID] = existing
}

func (e *Engine) markRunRetriesCancelled(runID uuid.UUID) {
	if runID == uuid.Nil {
		return
	}

	now := time.Now().UTC()
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pruneRetryStatesLocked(now)

	for retryID, state := range e.retryStates {
		if state.RunID != runID {
			continue
		}
		state.Status = "cancelled"
		state.UpdatedAt = now
		e.retryStates[retryID] = state
	}
}

func toRetryStatusItem(state retryState) models.RetryStatusItem {
	item := models.RetryStatusItem{
		RetryID:    state.RetryID,
		QuestionID: state.QuestionID,
		Status:     state.Status,
		Error:      state.Error,
		DurationMs: state.DurationMs,
		UpdatedAt:  state.UpdatedAt,
	}

	if state.RunID != uuid.Nil {
		item.RunID = state.RunID.String()
	}
	if state.AgentID != uuid.Nil {
		item.AgentID = state.AgentID.String()
	}
	if state.RunResultID != uuid.Nil {
		item.RunResultID = state.RunResultID.String()
	}

	return item
}

func (e *Engine) GetRetryStatus(workspaceID uuid.UUID, retryIDs []string) []models.RetryStatusItem {
	now := time.Now().UTC()
	out := make([]models.RetryStatusItem, 0, len(retryIDs))

	e.mu.Lock()
	defer e.mu.Unlock()

	e.pruneRetryStatesLocked(now)

	for _, retryID := range retryIDs {
		if retryID == "" {
			continue
		}

		state, ok := e.retryStates[retryID]
		if !ok {
			out = append(out, models.RetryStatusItem{
				RetryID:   retryID,
				Status:    "not_found",
				UpdatedAt: now,
			})
			continue
		}

		if workspaceID != uuid.Nil && state.WorkspaceID != uuid.Nil && state.WorkspaceID != workspaceID {
			out = append(out, models.RetryStatusItem{
				RetryID:   retryID,
				Status:    "not_found",
				UpdatedAt: now,
			})
			continue
		}

		out = append(out, toRetryStatusItem(state))
	}

	return out
}
