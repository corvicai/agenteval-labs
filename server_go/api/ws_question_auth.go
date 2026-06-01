package api

import (
	"encoding/json"
	"errors"
	"strings"

	dbcompat "benchmarking-platform/internal/db"
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// accessMode describes the relationship between a user and a question set.
type accessMode int

const (
	accessNone   accessMode = iota
	accessOwner             // user owns the workspace that contains the QS
	accessEditor            // user is an active collaborator with role='editor'
	accessViewer            // user is an active collaborator with role='viewer' (phase 2)
)

// getQuestionSetAccess returns the access level of userID on questionSetID.
//
//   - Owner: QS → client.workspace.user_id == userID
//   - Editor: active row in question_set_collaborators with role='editor'
//   - Viewer: active row in question_set_collaborators with role='viewer'
//
// The function also returns the loaded QuestionSet and its owning Workspace so
// callers don't need a second DB round-trip.
func (h *Hub) getQuestionSetAccess(db *gorm.DB, userID, questionSetID uuid.UUID) (accessMode, models.QuestionSet, models.Workspace, error) {
	qs, _, workspace, err := h.loadQuestionSetWithWorkspace(db, questionSetID)
	if err != nil {
		return accessNone, models.QuestionSet{}, models.Workspace{}, err
	}

	if workspace.UserID == userID {
		return accessOwner, qs, workspace, nil
	}

	// Check collaborator table.
	var collab models.QuestionSetCollaborator
	err = db.
		Where("question_set_id = ? AND user_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL", questionSetID, userID).
		First(&collab).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return accessNone, qs, workspace, nil
		}
		// If the table doesn't exist yet, treat as no access (schema not migrated).
		if dbcompat.IsMissingQuestionSetCollaboratorsRelationError(err) {
			return accessNone, qs, workspace, nil
		}
		return accessNone, qs, workspace, err
	}

	switch collab.Role {
	case "editor":
		return accessEditor, qs, workspace, nil
	case "viewer":
		return accessViewer, qs, workspace, nil
	default:
		return accessEditor, qs, workspace, nil
	}
}

func canReadQuestionSet(access accessMode) bool {
	return access == accessOwner || access == accessEditor || access == accessViewer
}

func canWriteQuestionSet(access accessMode) bool {
	return access == accessOwner || access == accessEditor
}

// sensitiveConfigKeys are substrings that identify sensitive agent config fields.
// Any config key whose lower-case form contains one of these is redacted.
var sensitiveConfigKeys = []string{"token", "api_key", "secret", "password"}

// redactAgentConfig returns a copy of the agent with sensitive config fields
// replaced by a masked value ("xx****xx" or "****"). Safe to call on agents
// from any workspace — used when collaborators (non-owners) need to see which
// agents are available without exposing credentials.
func redactAgentConfig(agent models.Agent) models.Agent {
	var cfg map[string]any
	if err := json.Unmarshal(agent.Config, &cfg); err != nil {
		// If we can't parse the config, return an agent with empty config.
		agent.Config = models.EncryptedJSON(`{}`)
		return agent
	}

	redacted := make(map[string]any, len(cfg))
	for k, v := range cfg {
		lower := strings.ToLower(k)
		isSensitive := false
		for _, sk := range sensitiveConfigKeys {
			if strings.Contains(lower, sk) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			if val, ok := v.(string); ok && len(val) > 4 {
				redacted[k] = val[:2] + "****" + val[len(val)-2:]
			} else {
				redacted[k] = "****"
			}
		} else {
			redacted[k] = v
		}
	}

	if b, err := json.Marshal(redacted); err == nil {
		agent.Config = models.EncryptedJSON(b)
	}
	return agent
}

// enrichQuestionSetSharing populates the transient sharing metadata on qs
// when the target user is an accepted collaborator (not the owner). For
// owners (or users with no access) it is a no-op — the QS is serialized
// exactly as it would have been before, preserving backwards compatibility.
//
// This centralizes what used to be a client-side inference (the `_shared`
// flag) so every handler that returns a QS to a user emits the same, server
// authoritative view: is_shared, owner_*, owner_agents (redacted), role.
// Frontend consumers should treat these fields as the source of truth.
//
// Errors are non-fatal — a best-effort log is emitted and the QS is returned
// without enrichment. Callers never need to branch on enrichment outcomes.
func (h *Hub) enrichQuestionSetSharing(db *gorm.DB, qs *models.QuestionSet, userID uuid.UUID) {
	if qs == nil || qs.ID == uuid.Nil || userID == uuid.Nil {
		return
	}

	access, _, ownerWorkspace, err := h.getQuestionSetAccess(db, userID, qs.ID)
	if err != nil {
		logger.Debug("[WS] enrichQuestionSetSharing: access lookup failed user=%s qs=%s err=%v",
			userID, qs.ID, err)
		return
	}
	if access == accessNone || access == accessOwner {
		return
	}

	var owner models.User
	if err := db.First(&owner, "id = ?", ownerWorkspace.UserID).Error; err != nil {
		logger.Warn("[WS] enrichQuestionSetSharing: could not load owner user=%s qs=%s err=%v",
			ownerWorkspace.UserID, qs.ID, err)
		return
	}

	var ownerAgents []models.Agent
	if err := db.Where("workspace_id = ?", ownerWorkspace.ID).
		Order("position ASC, created_at ASC").
		Find(&ownerAgents).Error; err != nil {
		logger.Warn("[WS] enrichQuestionSetSharing: could not load owner agents qs=%s err=%v",
			qs.ID, err)
	}
	redacted := make([]models.Agent, len(ownerAgents))
	for i, a := range ownerAgents {
		redacted[i] = redactAgentConfig(a)
	}

	var role string
	switch access {
	case accessEditor:
		role = "editor"
	case accessViewer:
		role = "viewer"
	default:
		role = "editor"
	}

	ownerUserID := ownerWorkspace.UserID
	ownerWsID := ownerWorkspace.ID

	qs.IsShared = true
	qs.OwnerUserID = &ownerUserID
	qs.OwnerName = owner.Name
	qs.OwnerWorkspaceID = &ownerWsID
	qs.OwnerAgents = redacted
	qs.Role = role
}
