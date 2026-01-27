package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"log"
)

// HandleWSMessage routes incoming messages to handlers
func (h *Hub) HandleWSMessage(c *Connection, env models.Envelope) {
	log.Printf("[WS] Handling message: %s (CorrelationID: %s)", env.Type, env.CorrelationID)

	switch env.Type {
	case ReqGetManagerStats:
		h.handleGetManagerStats(c, env)
	case ReqGetManagerUsers:
		h.handleGetManagerUsers(c, env)
	case ReqSyncState:
		h.handleSyncState(c, env)
	case ReqGetRunDetails:
		h.handleGetRunDetails(c, env)
	case ReqCreateEvaluation:
		h.handleCreateEvaluation(c, env)
	case ReqGetSpyPayload:
		h.handleGetSpyPayload(c, env)
	case ReqGetWorkspaceStats:
		h.handleGetWorkspaceStats(c, env)
	case ReqGetOrgStats:
		h.handleGetOrgStats(c, env)
	case ReqGetGlobalStats:
		h.handleGetGlobalStats(c, env)

	// Lazy Loading
	case ReqGetRunLite:
		h.handleGetRunLite(c, env)
	case ReqGetLatestRunByQS:
		h.handleGetLatestRunByQuestionSet(c, env)
	case ReqGetResultDetails:
		h.handleGetResultDetails(c, env)

	// Dev/Auth handlers
	case ReqDevGetManagers:
		h.handleDevGetManagers(c, env)
	case ReqDevLogin:
		h.handleDevLogin(c, env)
	case ReqGetWorkspaces:
		h.handleGetWorkspaces(c, env)
	case ReqCheckManagerStatus:
		h.handleCheckManagerStatus(c, env)
	case ReqGetMe:
		h.handleGetMe(c, env)
	case ReqCheckAdminExists:
		h.handleCheckAdminExists(c, env)
	case ReqWsLogin:
		h.handleWsLogin(c, env)
	case ReqAuth:
		h.handleAuth(c, env)
	case ReqAcceptTerms:
		h.handleAcceptTerms(c, env)
	case ReqGetWorkspaceRuns:
		h.handleGetWorkspaceRuns(c, env)
	case ReqSwitchWorkspace:
		h.handleSwitchWorkspace(c, env)
	case ReqCreateWorkspace:
		h.handleCreateWorkspace(c, env)
	case ReqCreateOrganization:
		h.handleCreateOrganization(c, env)
	case ReqJoinOrganization:
		h.handleJoinOrganization(c, env)
	case ReqCloneWorkspace:
		h.handleCloneWorkspace(c, env)
	case ReqDeleteRun:
		h.handleDeleteRun(c, env)
	case ReqDeleteAllRuns:
		h.handleDeleteAllRuns(c, env)
	case ReqGetWorkspaceClients:
		h.handleGetWorkspaceClients(c, env)
	case ReqUpdateAgent:
		h.handleUpdateAgent(c, env)
	case ReqImportQuestionSet:
		h.handleImportQuestionSet(c, env)
	case ReqExportQuestionSet:
		h.handleExportQuestionSet(c, env)
	case ReqUpdateQuestionSet:
		h.handleUpdateQuestionSet(c, env)
	case ReqCreateQuestionSet:
		h.handleCreateQuestionSet(c, env)
	case ReqUpdateQuestionSetAgents:
		h.handleUpdateQuestionSetAgents(c, env)
	case ReqCreateAgent:
		h.handleCreateAgent(c, env)
	case ReqDeleteAgent:
		h.handleDeleteAgent(c, env)
	case ReqWsRegister:
		h.handleWsRegister(c, env)
	case ReqWsBootstrapAdmin:
		h.handleWsBootstrapAdmin(c, env)

	// WebAuthn handlers
	case ReqWebAuthnDeleteKey:
		h.handleWebAuthnDeleteKey(c, env)

	// Admin handlers
	case ReqAdminGetUsers:
		h.handleAdminGetUsers(c, env)
	case ReqAdminGetOrganizations:
		h.handleAdminGetOrganizations(c, env)
	case ReqAdminGetUserProfile:
		h.handleAdminGetUserProfile(c, env)
	case ReqAdminGetOrgProfile:
		h.handleAdminGetOrgProfile(c, env)
	case ReqAdminCreateUser:
		h.handleAdminCreateUser(c, env)
	case ReqAdminCreateOrg:
		h.handleAdminCreateOrg(c, env)
	case ReqAdminUpdateUser:
		h.handleAdminUpdateUser(c, env)
	case ReqAdminDeleteUser:
		h.handleAdminDeleteUser(c, env)
	case ReqAdminUpdateOrg:
		h.handleAdminUpdateOrg(c, env)
	case ReqAdminDeleteOrg:
		h.handleAdminDeleteOrg(c, env)
	case ReqAdminGenerateInvite:
		h.handleAdminGenerateInvite(c, env)
	case ReqAdminRemoveUserFromOrg:
		h.handleAdminRemoveUserFromOrg(c, env)
	case ReqAdminGetLoginLogs:
		h.handleAdminGetLoginLogs(c, env)

	// Manager handlers
	case ReqManagerGetWorkspaces:
		h.handleManagerGetWorkspaces(c, env)
	case ReqManagerGetAgents:
		h.handleManagerGetAgents(c, env)
	case ReqManagerGetRuns:
		h.handleManagerGetRuns(c, env)
	case ReqManagerGetUsers:
		h.handleManagerGetUsers(c, env)
	case ReqManagerCreateUser:
		h.handleManagerCreateUser(c, env)
	case ReqManagerUpdateUser:
		h.handleManagerUpdateUser(c, env)
	case ReqManagerToggleUserSuspension:
		h.handleManagerToggleUserSuspension(c, env)
	case ReqManagerImpersonateUser:
		h.handleManagerImpersonateUser(c, env)
	case ReqManagerGetStats:
		h.handleManagerGetStats(c, env)
	case ReqManagerGenerateInvite:
		h.handleManagerGenerateInvite(c, env)

	case CmdStartRun:
		h.handleStartRun(c, env)
	case CmdRerunTask:
		h.handleRerunTask(c, env)
	case CmdCancelRun:
		h.handleCancelRun(c, env)
	case CmdRunEvaluators:
		h.handleRunEvaluators(c, env)
	case CmdSeedHistoricalRun:
		h.handleSeedHistoricalRun(c, env)
	case CmdAdminRecalculateStats:
		h.handleAdminRecalculateStats(c, env)
	case CmdAdminForceLogout:
		h.handleAdminForceLogout(c, env)
	case CmdAdminStartMaintenance:
		h.handleAdminStartMaintenance(c, env)
	case ReqCheckDBPerf:
		h.handleCheckDBPerf(c, env)

	default:
		// Authentication Guard
		if !c.IsAuthenticated {
			// Check if message is in the allowlist for unauthenticated connections
			switch env.Type {
			case ReqAuth, ReqCheckAdminExists, ReqWsBootstrapAdmin, ReqWsLogin, ReqWsRegister:
				// Allow these
			default:
				log.Printf("[WS] Rejected unauthenticated message: %s", env.Type)
				c.SendError(env.CorrelationID, "authentication required")
				return
			}
		}

		log.Printf("[WS] Unknown message type: %s", env.Type)
		c.SendError(env.CorrelationID, "unknown message type")
	}
}

func createJSONPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
