package api

import (
	"encoding/json"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

// handleGetMissedEvents serves REQ_GET_MISSED_EVENTS by replaying every buffered
// event the connection should have received since the supplied cursor.
//
// Audience filtering mirrors the broadcast fan-out:
//   - AudienceAll: always delivered
//   - AudienceWorkspace: connection workspace must match
//   - AudienceUser: connection user must match
//   - AudienceQS: user must own or actively collaborate on the question set
//
// If the buffer cannot guarantee completeness (server restart, unknown nonce,
// TTL expiry, or the event rotated out) the response sets NeedsFullSync=true
// and the client is expected to fall back to REQ_SYNC_STATE.
func (h *Hub) handleGetMissedEvents(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var payload models.GetMissedEventsPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	buffer := h.eventBuffer
	if buffer == nil {
		// Defensive: buffer should always exist after NewHub.
		c.SendResponse(DataMissedEvents, env.CorrelationID, models.MissedEventsResponse{
			NeedsFullSync: true,
		})
		return
	}

	if payload.SinceEventID == "" {
		// No cursor → nothing to replay; caller should have used SyncState.
		c.SendResponse(DataMissedEvents, env.CorrelationID, models.MissedEventsResponse{
			NeedsFullSync: true,
		})
		return
	}

	accessibleQS := h.resolveQuestionSetsForUser(c.UserID)
	accessibleAgents := h.resolveSharedAgentsForUser(c.UserID)

	filter := func(e BufferedEvent) bool {
		switch e.AudienceType {
		case AudienceAll:
			return true
		case AudienceWorkspace:
			return c.WorkspaceID != uuid.Nil && e.AudienceID == c.WorkspaceID
		case AudienceUser:
			return c.UserID != uuid.Nil && e.AudienceID == c.UserID
		case AudienceQS:
			if _, ok := accessibleQS[e.AudienceID]; ok {
				return true
			}
			return false
		case AudienceAgent:
			// AudienceAgent events carry the redacted variant targeted at
			// collaborators. Owners receive the full-fat copy via
			// AudienceUser, so only collaborators should match here.
			if _, ok := accessibleAgents[e.AudienceID]; ok {
				return true
			}
			return false
		default:
			return false
		}
	}

	events, lastEventID, needsFullSync := buffer.Since(payload.SinceEventID, filter)
	if needsFullSync {
		logger.Debug("[MISSED] conn=%s user=%s ws=%s cursor=%s → needs full sync",
			c.ID, c.UserID, c.WorkspaceID, payload.SinceEventID)
		c.SendResponse(DataMissedEvents, env.CorrelationID, models.MissedEventsResponse{
			NeedsFullSync: true,
		})
		return
	}

	rawEvents := make([]json.RawMessage, 0, len(events))
	for _, e := range events {
		rawEvents = append(rawEvents, append(json.RawMessage(nil), e.Msg...))
	}

	logger.Debug("[MISSED] conn=%s user=%s ws=%s cursor=%s → replayed %d event(s) lastId=%s",
		c.ID, c.UserID, c.WorkspaceID, payload.SinceEventID, len(rawEvents), lastEventID)

	c.SendResponse(DataMissedEvents, env.CorrelationID, models.MissedEventsResponse{
		NeedsFullSync: false,
		Events:        rawEvents,
		LastEventID:   lastEventID,
	})
}

// resolveQuestionSetsForUser returns the set of QS IDs the user may observe:
// every QS whose owner workspace belongs to them plus every QS they actively
// collaborate on. The result is a set so filter() lookups are O(1).
//
// This runs once per REQ_GET_MISSED_EVENTS — a single reconnect per client.
// Latency cost is a single pair of SELECTs, typically ~1 ms on a warm DB.
func (h *Hub) resolveQuestionSetsForUser(userID uuid.UUID) map[uuid.UUID]struct{} {
	result := make(map[uuid.UUID]struct{})
	if userID == uuid.Nil || h.db == nil {
		return result
	}

	type row struct {
		ID uuid.UUID `gorm:"column:id"`
	}

	var owned []row
	if err := h.db.Raw(`
		SELECT qs.id
		FROM question_sets qs
		JOIN clients cl ON cl.id = qs.client_id
		JOIN workspaces w ON w.id = cl.workspace_id
		WHERE w.user_id = ?
	`, userID).Scan(&owned).Error; err == nil {
		for _, r := range owned {
			result[r.ID] = struct{}{}
		}
	}

	type collabRow struct {
		QuestionSetID uuid.UUID `gorm:"column:question_set_id"`
	}
	var collabs []collabRow
	if err := h.db.Raw(`
		SELECT question_set_id
		FROM question_set_collaborators
		WHERE user_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL
	`, userID).Scan(&collabs).Error; err == nil {
		for _, r := range collabs {
			result[r.QuestionSetID] = struct{}{}
		}
	}

	return result
}

// resolveSharedAgentsForUser returns the set of Agent IDs the user has
// use-only access to (Plano 28). Owners are NOT included: they receive
// agent events via AudienceUser broadcasts with the full payload, while
// AudienceAgent envelopes only target collaborators.
//
// Schema-missing errors are swallowed so the feature degrades gracefully
// on databases that haven't run migration 006.
func (h *Hub) resolveSharedAgentsForUser(userID uuid.UUID) map[uuid.UUID]struct{} {
	result := make(map[uuid.UUID]struct{})
	if userID == uuid.Nil || h.db == nil {
		return result
	}

	type row struct {
		AgentID uuid.UUID `gorm:"column:agent_id"`
	}
	var rows []row
	_ = h.db.Raw(`
		SELECT agent_id
		FROM agent_collaborators
		WHERE user_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL
	`, userID).Scan(&rows).Error

	for _, r := range rows {
		result[r.AgentID] = struct{}{}
	}
	return result
}
