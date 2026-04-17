package api

import (
	"encoding/json"
	"errors"
	"strings"

	dbcompat "benchmarking-platform/internal/db"
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
