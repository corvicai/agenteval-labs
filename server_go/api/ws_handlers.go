package api

import (
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"
	"encoding/json"
)

// HandleWSMessage routes incoming messages to handlers
func (h *Hub) HandleWSMessage(c *Connection, env models.Envelope) {
	logger.Debug("[WS] Handling message: %s (CorrelationID: %s)", env.Type, env.CorrelationID)

	if !c.IsAuthenticated && !isWSMessageAllowedWithoutAuth(env.Type) {
		logger.Warn("[WS] Rejected unauthenticated message: %s", env.Type)
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

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
	case ReqGetRetryStatus:
		h.handleGetRetryStatus(c, env)
	case ReqGetMissedEvents:
		h.handleGetMissedEvents(c, env)
	case ReqPing:
		h.handlePing(c, env)
	case ReqGetRunProgress:
		h.handleGetRunProgress(c, env)
	case ReqGetPendingResponse:
		h.handleGetPendingResponse(c, env)

	// Run Comparison handlers
	case ReqCompareRuns:
		h.handleCompareRuns(c, env)
	case ReqCreateComparison:
		h.handleCreateComparison(c, env)
	case ReqListComparisons:
		h.handleListComparisons(c, env)
	case ReqGetComparison:
		h.handleGetComparison(c, env)
	case ReqDeleteComparison:
		h.handleDeleteComparison(c, env)
	case ReqListComparisonTemplates:
		h.handleListComparisonTemplates(c, env)
	case ReqCreateComparisonTemplate:
		h.handleCreateComparisonTemplate(c, env)
	case ReqUpdateComparisonTemplate:
		h.handleUpdateComparisonTemplate(c, env)
	case ReqDeleteComparisonTemplate:
		h.handleDeleteComparisonTemplate(c, env)

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
	case ReqDeleteQuestionSet:
		h.handleDeleteQuestionSet(c, env)
	case ReqCreateQuestionSetShareLink:
		h.handleCreateQuestionSetShareLink(c, env)
	case ReqGetQuestionSetShareLink:
		h.handleGetQuestionSetShareLink(c, env)
	case ReqAcceptQuestionSetShareLink:
		h.handleAcceptQuestionSetShareLink(c, env)

	// Collaborative Question Sets
	case ReqCreateQuestionSetCollabInvite:
		h.handleCreateQuestionSetCollabInvite(c, env)
	case ReqGetQuestionSetCollabInvite:
		h.handleGetQuestionSetCollabInvite(c, env)
	case ReqAcceptQuestionSetCollabInvite:
		h.handleAcceptQuestionSetCollabInvite(c, env)
	case ReqListQuestionSetCollaborators:
		h.handleListQuestionSetCollaborators(c, env)
	case ReqRevokeQuestionSetCollaborator:
		h.handleRevokeQuestionSetCollaborator(c, env)

	// Shared Agents (Plano 28)
	case ReqCreateAgentCollabInvite:
		h.handleCreateAgentCollabInvite(c, env)
	case ReqGetAgentCollabInvite:
		h.handleGetAgentCollabInvite(c, env)
	case ReqAcceptAgentCollabInvite:
		h.handleAcceptAgentCollabInvite(c, env)
	case ReqListAgentCollaborators:
		h.handleListAgentCollaborators(c, env)
	case ReqRevokeAgentCollaborator:
		h.handleRevokeAgentCollaborator(c, env)
	case ReqUpdateQuestionSetAgents:
		h.handleUpdateQuestionSetAgents(c, env)
	case ReqGetQuestionSetAgentEnvelope:
		h.handleGetQuestionSetAgentEnvelope(c, env)
	case ReqCreateAgent:
		h.handleCreateAgent(c, env)
	case ReqDeleteAgent:
		h.handleDeleteAgent(c, env)
	case ReqWsRegister:
		h.handleWsRegister(c, env)
	case ReqWsBootstrapAdmin:
		h.handleWsBootstrapAdmin(c, env)
	case ReqChangePassword:
		h.handleWsChangePassword(c, env)

	// WebAuthn handlers
	case ReqWebAuthnRegisterBegin:
		h.handleWebAuthnRegisterBegin(c, env)
	case ReqWebAuthnRegisterFinish:
		h.handleWebAuthnRegisterFinish(c, env)
	case ReqWebAuthnLoginBegin:
		h.handleWebAuthnLoginBegin(c, env)
	case ReqWebAuthnLoginFinish:
		h.handleWebAuthnLoginFinish(c, env)
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
	case ReqAdminGetRuns:
		h.handleAdminGetRuns(c, env)
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
	case ReqAdminGetDebugInfo:
		h.handleAdminGetDebugInfo(c, env)

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
		logger.Warn("[WS] Unknown message type: %s", env.Type)
		c.SendError(env.CorrelationID, "unknown message type")
	}
}

func isWSMessageAllowedWithoutAuth(messageType string) bool {
	switch messageType {
	case ReqAuth,
		ReqCheckAdminExists,
		ReqWsBootstrapAdmin,
		ReqWsLogin,
		ReqWsRegister,
		ReqWebAuthnLoginBegin,
		ReqWebAuthnLoginFinish,
		ReqDevGetManagers,
		ReqDevLogin,
		ReqPing:
		return true
	default:
		return false
	}
}

func createJSONPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
