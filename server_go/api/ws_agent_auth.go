package api

import (
	"errors"

	dbcompat "benchmarking-platform/internal/db"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// agentAccessMode describes the relationship between a user and an Agent.
// See Plano 28 (Shared Agents) in docs/improvement-plans.md.
type agentAccessMode int

const (
	// agentAccessNone — the user has no relationship with the agent.
	agentAccessNone agentAccessMode = iota
	// agentAccessOwner — the user owns the workspace that contains the agent.
	agentAccessOwner
	// agentAccessUser — the user is an active collaborator with use-only access.
	agentAccessUser
)

// getAgentAccess returns the access level of userID on agentID.
//
//   - Owner: agent.workspace.user_id == userID
//   - User (collaborator): active row in agent_collaborators
//     (accepted_at != NULL, revoked_at == NULL)
//
// The function also returns the loaded Agent and owning Workspace so callers
// don't need a second DB round-trip. When access == agentAccessNone the Agent
// and Workspace may still be populated (e.g. agent exists but user has no
// relationship) — callers should inspect both values.
func (h *Hub) getAgentAccess(db *gorm.DB, userID, agentID uuid.UUID) (agentAccessMode, models.Agent, models.Workspace, error) {
	var agent models.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		return agentAccessNone, agent, models.Workspace{}, err
	}

	var workspace models.Workspace
	if err := db.First(&workspace, "id = ?", agent.WorkspaceID).Error; err != nil {
		return agentAccessNone, agent, workspace, err
	}

	if workspace.UserID == userID {
		return agentAccessOwner, agent, workspace, nil
	}

	var collab models.AgentCollaborator
	err := db.
		Where("agent_id = ? AND user_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL", agentID, userID).
		First(&collab).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agentAccessNone, agent, workspace, nil
		}
		if dbcompat.IsMissingAgentCollaboratorsRelationError(err) {
			// Schema not migrated yet → treat as no access but don't surface
			// the error to callers (graceful degradation).
			return agentAccessNone, agent, workspace, nil
		}
		return agentAccessNone, agent, workspace, err
	}

	return agentAccessUser, agent, workspace, nil
}
