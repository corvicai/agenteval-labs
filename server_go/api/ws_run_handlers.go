package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"benchmarking-platform/internal/logger"

	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// injectEvaluatorTargetIDsLite fills TargetRunResultID for evaluator
// RunResultLite entries using a lookup over the primary results already present
// in the slice. No extra DB round-trip is required.
func injectEvaluatorTargetIDsLite(results []models.RunResultLite) {
	primary := make(map[string]uuid.UUID, len(results))
	for _, r := range results {
		if !strings.HasPrefix(r.QuestionID, "eval-") {
			primary[r.AgentID.String()+":"+r.QuestionID] = r.ID
		}
	}
	for i := range results {
		if !strings.HasPrefix(results[i].QuestionID, "eval-") {
			continue
		}
		rest := strings.TrimPrefix(results[i].QuestionID, "eval-")
		// Format: <36-char UUID> + "-" + questionID
		if len(rest) <= 36 {
			continue
		}
		targetAgentStr := rest[:36]
		questionID := rest[37:]
		if primaryID, ok := primary[targetAgentStr+":"+questionID]; ok {
			id := primaryID
			results[i].TargetRunResultID = &id
		}
	}
}

// injectEvaluatorTargetIDsFull is the same but for full RunResult slices
// (used by handleGetRunDetails).
func injectEvaluatorTargetIDsFull(results []models.RunResult) {
	primary := make(map[string]uuid.UUID, len(results))
	for _, r := range results {
		if !strings.HasPrefix(r.QuestionID, "eval-") {
			primary[r.AgentID.String()+":"+r.QuestionID] = r.ID
		}
	}
	for i := range results {
		if !strings.HasPrefix(results[i].QuestionID, "eval-") {
			continue
		}
		rest := strings.TrimPrefix(results[i].QuestionID, "eval-")
		if len(rest) <= 36 {
			continue
		}
		targetAgentStr := rest[:36]
		questionID := rest[37:]
		if primaryID, ok := primary[targetAgentStr+":"+questionID]; ok {
			id := primaryID
			results[i].TargetRunResultID = &id
		}
	}
}

func normalizeResultEvaluationsForDisplay(result *models.RunResult) {
	if result == nil || len(result.Evaluations) == 0 {
		return
	}

	hasUserEval := false
	var latestAgentEval *models.Evaluation

	for i := range result.Evaluations {
		ev := result.Evaluations[i]
		raterType := strings.ToLower(strings.TrimSpace(ev.RaterType))
		if raterType == "user" {
			hasUserEval = true
			break
		}
		if raterType != "agent" {
			continue
		}
		if latestAgentEval == nil || ev.CreatedAt.After(latestAgentEval.CreatedAt) {
			cloned := ev
			latestAgentEval = &cloned
		}
	}

	if hasUserEval || latestAgentEval == nil {
		return
	}

	// Backward-compatibility bridge for clients that only read rater_type=user.
	synthetic := *latestAgentEval
	// Legacy auto-evaluator runs stored low scores as "wrong", which the UI
	// interprets as partial. Normalize those to negative for display.
	if strings.EqualFold(strings.TrimSpace(synthetic.Rating), "wrong") && synthetic.Score != nil && *synthetic.Score <= 50 {
		ratingCode := 3
		synthetic.Rating = "dislike"
		synthetic.RatingCode = &ratingCode
	}
	synthetic.ID = uuid.New()
	synthetic.RaterType = "user"
	synthetic.RaterID = uuid.Nil
	result.Evaluations = append(result.Evaluations, synthetic)
}

func normalizeResultsEvaluationsForDisplay(results []models.RunResult) {
	for i := range results {
		normalizeResultEvaluationsForDisplay(&results[i])
	}
}

func ensureAgentSyncConfigs(agents []models.Agent) (hadDecryptionErrors bool) {
	for i := range agents {
		if len(agents[i].Config) == 0 {
			agents[i].Config = models.EncryptedJSON([]byte(`{}`))
			continue
		}
		var m map[string]any
		if json.Unmarshal(agents[i].Config, &m) == nil {
			if _, bad := m["_error"]; bad {
				agents[i].Config = models.EncryptedJSON([]byte(`{}`))
				agents[i].ConfigStatus = "needs_recredentials"
				hadDecryptionErrors = true
			}
		}
	}
	return
}


func (h *Hub) handleStartRun(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var payload models.StartRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	var agentIDs []uuid.UUID
	seenAgentIDs := make(map[uuid.UUID]struct{}, len(payload.AgentIDs))
	for _, idStr := range payload.AgentIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid agent_id")
			return
		}
		if _, seen := seenAgentIDs[id]; !seen {
			seenAgentIDs[id] = struct{}{}
			agentIDs = append(agentIDs, id)
		}
	}

	access, _, ownerWs, accessErr := h.getQuestionSetAccess(h.db, c.UserID, qsID)
	if accessErr != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	if !canWriteQuestionSet(access) {
		c.SendError(env.CorrelationID, "access denied")
		return
	}

	// Validate Agent Credentials
	if len(agentIDs) > 0 {
		var agents []models.Agent
		if err := h.db.Find(&agents, agentIDs).Error; err != nil {
			c.SendError(env.CorrelationID, "failed to load agents for validation")
			return
		}
		if len(agents) != len(agentIDs) {
			c.SendError(env.CorrelationID, "one or more selected agents were not found")
			return
		}

		// Authorize every selected agent (Plano 28). Each must be either
		// owned by the user OR shared with them via agent_collaborators.
		// This prevents a client from smuggling in a stranger's agent ID.
		for _, agent := range agents {
			access, _, _, accessErr := h.getAgentAccess(h.db, c.UserID, agent.ID)
			if accessErr != nil {
				c.SendError(env.CorrelationID, fmt.Sprintf("failed to verify access to agent '%s'", agent.Name))
				return
			}
			if access == agentAccessNone {
				c.SendError(env.CorrelationID, fmt.Sprintf("agent '%s' is not accessible to you", agent.Name))
				return
			}
		}

		for _, agent := range agents {
			// A poisoned config means the stored ciphertext no longer decrypts
			// (encryption key changed). Surface the real cause for ANY provider
			// instead of falling through to a misleading "missing credentials".
			if models.ConfigDecryptionFailed(agent.Config) {
				c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' credentials could not be decrypted (the encryption key changed). Please re-enter its credentials in Agent Settings.", agent.Name))
				return
			}

			// Skip disabled agents? UI sends selected agents. We validate all selected.
			var config map[string]interface{}
			if err := json.Unmarshal(agent.Config, &config); err != nil {
				continue
			}

			// Mock/dry-run credentials are simulated in dev but refused at
			// execution in production. Reject at start with a clear message
			// instead of queueing tasks that will all fail.
			if orchestrator.IsProductionAppEnv() && orchestrator.ConfigUsesMockCredential(config) {
				c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' uses a MOCK/DRYRUN credential, which does not run in production. Set a real API key in Agent Settings.", agent.Name))
				return
			}

			switch agent.ProviderType {
			case "openai":
				apiKey, _ := config["api_key"].(string)
				if strings.TrimSpace(apiKey) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (API Key is required)", agent.Name))
					return
				}
				promptID, _ := config["prompt_id"].(string)
				mode, _ := config["openai_mode"].(string)
				mode = strings.ToLower(strings.TrimSpace(mode))
				if mode == "" {
					if strings.TrimSpace(promptID) != "" {
						mode = "managed"
					} else {
						mode = "standard"
					}
				}
				if mode == "managed" && strings.TrimSpace(promptID) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is in managed mode but Prompt ID is missing", agent.Name))
					return
				}
			case "nvidia":
				apiKey, _ := config["api_key"].(string)
				if strings.TrimSpace(apiKey) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (NVIDIA API Key is required)", agent.Name))
					return
				}
			case "anthropic":
				apiKey, _ := config["anthropic_api_key"].(string)
				if strings.TrimSpace(apiKey) == "" {
					apiKey, _ = config["api_key"].(string)
				}
				if strings.TrimSpace(apiKey) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (anthropic_api_key or api_key is required)", agent.Name))
					return
				}
			case "openrouter":
				apiKey, _ := config["openrouter_api_key"].(string)
				if strings.TrimSpace(apiKey) == "" {
					apiKey, _ = config["api_key"].(string)
				}
				if strings.TrimSpace(apiKey) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (openrouter_api_key or api_key is required)", agent.Name))
					return
				}
			case "openai_compatible":
				apiKey, _ := config["compatible_api_key"].(string)
				if strings.TrimSpace(apiKey) == "" {
					apiKey, _ = config["api_key"].(string)
				}
				baseURL, _ := config["compatible_base_url"].(string)
				if strings.TrimSpace(baseURL) == "" {
					baseURL, _ = config["base_url"].(string)
				}
				if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(baseURL) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (compatible_api_key/api_key and compatible_base_url/base_url are required)", agent.Name))
					return
				}
			case "evaluator":
				preferredProvider := orchestrator.PreferredEvaluatorProvider(config)
				resolvedProvider := orchestrator.ResolveEvaluatorProvider(config)

				if !orchestrator.IsSelectedEvaluatorProviderConfigured(config) {
					switch preferredProvider {
					case orchestrator.EvaluatorProviderNVIDIA:
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (nvidia_api_key or api_key is required)", agent.Name))
					case orchestrator.EvaluatorProviderOpenRouter:
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (openrouter_api_key or api_key is required)", agent.Name))
					case orchestrator.EvaluatorProviderAnthropic:
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (anthropic_api_key or api_key is required)", agent.Name))
					case orchestrator.EvaluatorProviderOpenAICompatible:
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (compatible_api_key + compatible_base_url are required)", agent.Name))
					case orchestrator.EvaluatorProviderAuto:
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing evaluator credentials (configure at least one: nvidia_api_key, openrouter_api_key, anthropic_api_key, openai_api_key or compatible_api_key+compatible_base_url)", agent.Name))
					default:
						if orchestrator.EvaluatorOpenAIMode(config) == "managed" {
							c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is in managed mode but Prompt ID is missing", agent.Name))
						} else {
							c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (openai_api_key or api_key is required)", agent.Name))
						}
					}
					return
				}

				if (preferredProvider == orchestrator.EvaluatorProviderOpenAI || preferredProvider == orchestrator.EvaluatorProviderAuto) &&
					resolvedProvider == orchestrator.EvaluatorProviderOpenAI &&
					orchestrator.EvaluatorOpenAIMode(config) == "managed" &&
					strings.TrimSpace(orchestrator.EvaluatorOpenAIPromptID(config)) == "" {
					c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is in managed mode but Prompt ID is missing", agent.Name))
					return
				}
			case "mcp":
				mode, _ := config["mode"].(string)
				if mode == "http" || mode == "" {
					endpoint, _ := config["endpoint"].(string)
					token, _ := config["token"].(string)
					// User requested check for BOTH endpoint and token
					if endpoint == "" || token == "" {
						c.SendError(env.CorrelationID, fmt.Sprintf("Agent '%s' is missing credentials (Endpoint and Token are required)", agent.Name))
						return
					}
				}
			}
		}
	}

	const runnerPingTimeout = 5 * time.Minute
	logger.Debug("[RUNNER] Ping: are you there? (timeout=%s)", runnerPingTimeout)
	if err := h.engine.PingRunner(); err != nil {
		logger.Error("[RUNNER] Ping failed: %v", err)
		c.SendError(env.CorrelationID, "Runner sounds offline. Please try again in a moment.")
		return
	}
	logger.Debug("[RUNNER] Ping OK: I'm here")

	// For shared QSs, runs must live in the owner's workspace so they're
	// scoped correctly (agents, broadcasts, stats, later reads all live
	// there). MUST stay in sync with handleGetLatestRunByQuestionSet /
	// handleGetRunDetails / handleSyncState, otherwise collaborators will
	// create runs they can't read back (they'd query the owner's workspace
	// while the run sits orphaned in the collaborator's workspace).
	runWorkspaceID := c.WorkspaceID
	if access != accessOwner {
		runWorkspaceID = ownerWs.ID
		logger.Debug("[WS] handleStartRun: routing shared-QS run to owner workspace user=%s qs=%s access=%d ownerWs=%s",
			c.UserID, qsID, access, ownerWs.ID)
	}
	run, err := h.engine.StartRunForUser(runWorkspaceID, qsID, agentIDs, c.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	// Notify the full QS audience (owner + collaborators) that a run was
	// created. Without this, collaborators never learn about the new run and
	// never see live progress/results until they trigger a fresh syncState.
	// Falls back to workspace-scoped broadcast if QS fan-out fails.
	createdPayload := models.DataChangedPayload{
		Resource: "runs",
		Action:   "created",
		Data:     run,
	}
	if sendErr := h.SendEventToQS(qsID, EvtDataChanged, "", createdPayload); sendErr != nil {
		logger.Warn("[WS] handleStartRun: QS fan-out failed for run %s: %v", run.ID, sendErr)
		h.BroadcastEvent(runWorkspaceID, "runs", "created", run)
	}

	if err := h.cacheAndSendResponse(c, DataResponse, env.CorrelationID, run); err != nil {
		logger.Warn("[WS] handleStartRun: send failed: %v", err)
	}
}

func (h *Hub) handleRerunTask(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var payload models.RerunTaskPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, err := uuid.Parse(payload.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}
	agentID, err := uuid.Parse(payload.AgentID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent_id")
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

	// Authorize agent access (Plano 28). Users can rerun a task only with an
	// agent they own or that has been shared with them.
	if access, _, _, accessErr := h.getAgentAccess(h.db, c.UserID, agentID); accessErr != nil {
		c.SendError(env.CorrelationID, "failed to verify agent access")
		return
	} else if access == agentAccessNone {
		c.SendError(env.CorrelationID, "agent is not accessible to you")
		return
	}

	retryID := uuid.NewString()

	// Pass frontend-provided context to engine
	opts := &orchestrator.RerunTaskOptions{
		OriginalQuestion: payload.OriginalQuestion,
		ExpectedAnswer:   payload.ExpectedAnswer,
		QuestionSetID:    payload.QuestionSetID,
		ResultID:         payload.ResultID,
		RetryID:          retryID,
	}

	if err := h.engine.RerunTask(runID, agentID, payload.QuestionID, opts); err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	if err := h.cacheAndSendResponse(c, DataResponse, env.CorrelationID, map[string]string{
		"status":   "queued",
		"retry_id": retryID,
	}); err != nil {
		logger.Warn("[WS] handleRerunTask: send failed: %v", err)
	}
}

func (h *Hub) handleCancelRun(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var payload models.CancelRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, err := uuid.Parse(payload.RunID)
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

	h.engine.CancelRun(runID)
	if err := h.cacheAndSendResponse(c, DataResponse, env.CorrelationID, map[string]string{"status": "cancelled"}); err != nil {
		logger.Warn("[WS] handleCancelRun: send failed: %v", err)
	}
}

func (h *Hub) handleSyncState(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated || c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}
	if c.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "no workspace selected")
		return
	}
	if _, err := h.loadOwnedWorkspace(h.db, c.UserID, c.WorkspaceID); err != nil {
		c.SendError(env.CorrelationID, "workspace not found or access denied")
		return
	}

	var payload models.SyncStatePayload

	logger.Debug("[WS] SyncState requested for workspace: %s", c.WorkspaceID)

	// 1. Get Agents
	if err := h.db.Where("workspace_id = ?", c.WorkspaceID).Order("created_at desc").Find(&payload.Agents).Error; err != nil {
		logger.Warn("[WS] SyncState full agent load failed, retrying without encrypted config: %v", err)
		payload.Warnings = append(payload.Warnings, "Some agent configs could not be decrypted; returning agent metadata without config.")

		if fallbackErr := h.db.Model(&models.Agent{}).
			Select("id", "workspace_id", "name", "provider_type", "enabled", "position", "max_concurrency", "created_at").
			Where("workspace_id = ?", c.WorkspaceID).
			Order("created_at desc").
			Find(&payload.Agents).Error; fallbackErr != nil {
			logger.Error("[WS] SyncState fallback agent load failed: %v", fallbackErr)
			c.SendError(env.CorrelationID, "failed to load agents: "+fallbackErr.Error())
			return
		}
		ensureAgentSyncConfigs(payload.Agents)
	} else if ensureAgentSyncConfigs(payload.Agents) {
		payload.Warnings = append(payload.Warnings, "Some agent configs could not be decrypted; returning agent metadata without config.")
	}

	// 1b. Get Shared Agents (Plano 28) — agents the user was granted
	// use-only access to. Config is redacted before serialization so
	// credentials never leave the backend.
	type sharedAgentRow struct {
		AgentID     uuid.UUID `gorm:"column:agent_id"`
		AcceptedAt  time.Time `gorm:"column:accepted_at"`
		OwnerUserID uuid.UUID `gorm:"column:owner_user_id"`
		OwnerName   string    `gorm:"column:owner_name"`
	}
	var sharedAgentRows []sharedAgentRow
	sharedAgentErr := h.db.Raw(`
		SELECT ac.agent_id, ac.accepted_at,
		       w.user_id AS owner_user_id,
		       u.name    AS owner_name
		FROM agent_collaborators ac
		JOIN agents a ON a.id = ac.agent_id
		JOIN workspaces w ON w.id = a.workspace_id
		JOIN users u ON u.id = w.user_id
		WHERE ac.user_id = ? AND ac.accepted_at IS NOT NULL AND ac.revoked_at IS NULL
		ORDER BY ac.accepted_at DESC
	`, c.UserID).Scan(&sharedAgentRows).Error

	if sharedAgentErr == nil && len(sharedAgentRows) > 0 {
		payload.SharedAgents = make([]models.SharedAgent, 0, len(sharedAgentRows))
		for _, row := range sharedAgentRows {
			var agent models.Agent
			if loadErr := h.db.First(&agent, "id = ?", row.AgentID).Error; loadErr != nil {
				logger.Warn("[WS] SyncState: could not load shared agent %s: %v", row.AgentID, loadErr)
				continue
			}
			agent = redactAgentConfig(agent)
			payload.SharedAgents = append(payload.SharedAgents, models.SharedAgent{
				Agent:       agent,
				OwnerUserID: row.OwnerUserID,
				OwnerName:   row.OwnerName,
				AcceptedAt:  row.AcceptedAt,
			})
		}
	} else if sharedAgentErr != nil {
		// Missing table (schema not migrated) is expected on fresh dev DBs — ignore.
		logger.Debug("[WS] SyncState: could not load shared agents (may not be migrated): %v", sharedAgentErr)
	}

	// 2. Get Question Sets
	if err := h.db.Model(&models.QuestionSet{}).
		Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("clients.workspace_id = ?", c.WorkspaceID).
		Preload("Client").
		Preload("Agents").
		Order("question_sets.created_at desc").
		Find(&payload.QuestionSets).Error; err != nil {
		logger.Warn("[WS] SyncState full question set load failed, retrying without agent overrides: %v", err)
		payload.Warnings = append(payload.Warnings, "Some question set agent overrides could not be decrypted; returning question sets without embedded agent overrides.")

		if fallbackErr := h.db.Model(&models.QuestionSet{}).
			Joins("JOIN clients ON clients.id = question_sets.client_id").
			Where("clients.workspace_id = ?", c.WorkspaceID).
			Preload("Client").
			Order("question_sets.created_at desc").
			Find(&payload.QuestionSets).Error; fallbackErr != nil {
			logger.Error("[WS] SyncState fallback question set load failed: %v", fallbackErr)
			c.SendError(env.CorrelationID, "failed to load question sets: "+fallbackErr.Error())
			return
		}
	} else {
		// Filter out QuestionSetAgent entries with undecryptable configs (_error marker)
		var hadBadOverrides bool
		for i := range payload.QuestionSets {
			clean := payload.QuestionSets[i].Agents[:0]
			for _, qsa := range payload.QuestionSets[i].Agents {
				var m map[string]any
				if json.Unmarshal(qsa.Config, &m) == nil {
					if _, bad := m["_error"]; bad {
						hadBadOverrides = true
						continue
					}
				}
				clean = append(clean, qsa)
			}
			payload.QuestionSets[i].Agents = clean
		}
		if hadBadOverrides {
			payload.Warnings = append(payload.Warnings, "Some question set agent overrides could not be decrypted; returning question sets without embedded agent overrides.")
		}
	}

	// 2b. Get Shared Question Sets (QSs owned by other users where current user is an active collaborator)
	type sharedQSRow struct {
		// question_sets columns
		ID       string `gorm:"column:id"`
		ClientID string `gorm:"column:client_id"`
		Name     string `gorm:"column:name"`
		Version  string `gorm:"column:version"`
		// owner info
		OwnerUserID      string `gorm:"column:owner_user_id"`
		OwnerName        string `gorm:"column:owner_name"`
		OwnerWorkspaceID string `gorm:"column:owner_workspace_id"`
		// collaborator info
		Role       string     `gorm:"column:role"`
		AcceptedAt *time.Time `gorm:"column:accepted_at"`
	}
	var sharedRows []sharedQSRow
	sharedErr := h.db.Raw(`
		SELECT
			qs.id, qs.client_id, qs.name, qs.version,
			w.user_id AS owner_user_id,
			u.name AS owner_name,
			w.id AS owner_workspace_id,
			c.role, c.accepted_at
		FROM question_set_collaborators c
		JOIN question_sets qs ON qs.id = c.question_set_id
		JOIN clients cl ON cl.id = qs.client_id
		JOIN workspaces w ON w.id = cl.workspace_id
		JOIN users u ON u.id = w.user_id
		WHERE c.user_id = ? AND c.accepted_at IS NOT NULL AND c.revoked_at IS NULL
		ORDER BY c.accepted_at DESC
	`, c.UserID).Scan(&sharedRows).Error

	// Collect shared QS IDs for run query and OwnerAgents loading below.
	sharedQSIDs := make([]uuid.UUID, 0)

	if sharedErr == nil && len(sharedRows) > 0 {
		payload.SharedQuestionSets = make([]models.SharedQuestionSet, 0, len(sharedRows))
		for _, row := range sharedRows {
			qsID, qsIDErr := uuid.Parse(row.ID)
			ownerUserID, ownerUserErr := uuid.Parse(row.OwnerUserID)
			ownerWsID, ownerWsErr := uuid.Parse(row.OwnerWorkspaceID)
			if qsIDErr != nil || ownerUserErr != nil || ownerWsErr != nil {
				logger.Warn("[WS] SyncState: skipping shared QS row with unparseable UUIDs: qs=%q user=%q ws=%q",
					row.ID, row.OwnerUserID, row.OwnerWorkspaceID)
				continue
			}
			// Load the full QS (with Client + Agents)
			var fullQS models.QuestionSet
			if loadErr := h.db.Preload("Client").Preload("Agents").First(&fullQS, "id = ?", qsID).Error; loadErr != nil {
				logger.Warn("[WS] SyncState: could not load shared QS %s: %v", qsID, loadErr)
				continue
			}
			at := time.Time{}
			if row.AcceptedAt != nil {
				at = *row.AcceptedAt
			}

			// Load owner's workspace agents (redacted) so the collaborator can
			// select them when starting a run — secrets are never exposed.
			var ownerAgents []models.Agent
			if loadErr := h.db.Where("workspace_id = ?", ownerWsID).
				Order("position ASC, created_at ASC").
				Find(&ownerAgents).Error; loadErr != nil {
				logger.Warn("[WS] SyncState: could not load owner agents for shared QS %s: %v", qsID, loadErr)
			}
			redactedOwnerAgents := make([]models.Agent, len(ownerAgents))
			for i, a := range ownerAgents {
				redactedOwnerAgents[i] = redactAgentConfig(a)
			}

			// Populate the transient sharing metadata on the embedded QS too,
			// so clients can treat any SharedQuestionSet exactly like an
			// enriched standalone QuestionSet (is_shared=true, owner_agents,
			// role, …) without special-casing the sidebar payload shape.
			fullQS.IsShared = true
			sharedOwnerUserID := ownerUserID
			sharedOwnerWsID := ownerWsID
			fullQS.OwnerUserID = &sharedOwnerUserID
			fullQS.OwnerName = row.OwnerName
			fullQS.OwnerWorkspaceID = &sharedOwnerWsID
			fullQS.OwnerAgents = redactedOwnerAgents
			fullQS.Role = row.Role

			payload.SharedQuestionSets = append(payload.SharedQuestionSets, models.SharedQuestionSet{
				QuestionSet:      fullQS,
				OwnerUserID:      ownerUserID,
				OwnerName:        row.OwnerName,
				OwnerWorkspaceID: ownerWsID,
				Role:             row.Role,
				AcceptedAt:       at,
				OwnerAgents:      redactedOwnerAgents,
			})
			sharedQSIDs = append(sharedQSIDs, qsID)
		}
	} else if sharedErr != nil {
		// If the table doesn't exist yet, silently ignore (schema not migrated).
		logger.Debug("[WS] SyncState: could not load shared question sets (may not be migrated): %v", sharedErr)
	}

	// 3. Get Recent Runs — own workspace + runs belonging to shared QSs
	// (which live in the owner's workspace, not the collaborator's).
	var runsErr error
	if len(sharedQSIDs) > 0 {
		runsErr = h.db.Raw(`
			SELECT r.*, qs.name as question_set_name
			FROM runs r
			JOIN question_sets qs ON r.question_set_id = qs.id
			WHERE r.workspace_id = ? OR r.question_set_id IN ?
			ORDER BY r.created_at desc
			LIMIT 20
		`, c.WorkspaceID, sharedQSIDs).Scan(&payload.RecentRuns).Error
	} else {
		runsErr = h.db.Raw(`
			SELECT r.*, qs.name as question_set_name
			FROM runs r
			JOIN question_sets qs ON r.question_set_id = qs.id
			WHERE r.workspace_id = ?
			ORDER BY r.created_at desc
			LIMIT 10
		`, c.WorkspaceID).Scan(&payload.RecentRuns).Error
	}
	if runsErr != nil {
		logger.Error("[WS] SyncState error loading recent runs: %v", runsErr)
		c.SendError(env.CorrelationID, "failed to load recent runs: "+runsErr.Error())
		return
	}

	// 4. Hydrate active run results so the frontend can restore in-progress UI
	// state after an AFK/network reconnect without a separate REQ_GET_RUN_DETAILS.
	// Pick the most recent run that is still running or pending.
	for _, run := range payload.RecentRuns {
		if run.Status != "running" && run.Status != "pending" {
			continue
		}
		var results []models.RunResult
		if err := h.db.
			Preload("Evaluations").
			Where("run_id = ?", run.ID).
			Order("created_at asc").
			Limit(500).
			Find(&results).Error; err != nil {
			logger.Warn("[WS] SyncState: failed to hydrate active run %s results: %v", run.ID, err)
			payload.Warnings = append(payload.Warnings, "active run results could not be loaded")
			break
		}
		// Normalize agent-only evaluations into the user-rating shape the
		// frontend consumes, matching the behaviour of REQ_GET_RUN_DETAILS.
		normalizeResultsEvaluationsForDisplay(results)
		payload.ActiveRunHydration = &models.ActiveRunHydration{
			RunID:         run.ID,
			TotalExpected: run.TotalTasks,
			Results:       results,
		}
		logger.Debug("[WS] SyncState: hydrated active run %s with %d results (total_expected=%d)",
			run.ID, len(results), run.TotalTasks)
		break // only hydrate the most recent active run
	}

	logger.Debug("[WS] SyncState completed for workspace: %s (Agents: %d, Sets: %d, Runs: %d)",
		c.WorkspaceID, len(payload.Agents), len(payload.QuestionSets), len(payload.RecentRuns))

	// Include encryption health for admin users so the admin panel can show a
	// persistent banner without the operator needing to navigate to the debug tab.
	if c.UserID != uuid.Nil && h.db != nil {
		var user models.User
		if err := h.db.Select("is_admin, email").First(&user, "id = ?", c.UserID).Error; err == nil && user.HasAdminAccess() {
			if encHealth, ehErr := encryptionKeyHealthForAdmin(h.db); ehErr == nil && encHealth != nil {
				payload.EncryptionHealth = encHealth
			}
		}
	}

	c.SendResponse(DataState, env.CorrelationID, payload)
}

func (h *Hub) handleGetRunDetails(c *Connection, env models.Envelope) {
	var req models.GetRunDetailsPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	runID, err := uuid.Parse(req.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	var response models.RunDetailsResponse

	// 1. Get Run
	if err := h.db.First(&response.Run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Authorize: only the question set's owner or an accepted collaborator may
	// read this run (prevents cross-tenant IDOR via guessed run IDs).
	if access, _, _, aerr := h.getQuestionSetAccess(h.db, c.UserID, response.Run.QuestionSetID); aerr != nil || !canReadQuestionSet(access) {
		c.SendError(env.CorrelationID, "access denied")
		return
	}

	// 2. Get Question Set
	if err := h.db.Preload("Client").Preload("Agents").First(&response.QuestionSet, "id = ?", response.Run.QuestionSetID).Error; err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	// Attach server-authoritative sharing metadata (is_shared, owner_agents,
	// role, …) so collaborators see the correct agent list on the client
	// without relying on any frontend-side flag.
	h.enrichQuestionSetSharing(h.db, &response.QuestionSet, c.UserID)

	// 3. Get Results (including evaluations)
	if err := h.db.Preload("Evaluations").
		Where("run_id = ?", runID).
		Order("created_at ASC").
		Order("id ASC").
		Find(&response.Results).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to load results: "+err.Error())
		return
	}
	response.Results = orchestrator.CollapseRunResultsToLatest(response.Results)
	injectEvaluatorTargetIDsFull(response.Results)
	normalizeResultsEvaluationsForDisplay(response.Results)

	// 4. Collect Agent info
	response.Agents = make(map[string]models.Agent)
	for _, res := range response.Results {
		if _, exists := response.Agents[res.AgentID.String()]; !exists {
			var agent models.Agent
			if err := h.db.First(&agent, "id = ?", res.AgentID).Error; err == nil {
				response.Agents[res.AgentID.String()] = agent
			}
		}
	}

	c.SendResponse(DataRunDetails, env.CorrelationID, response)
}

func (h *Hub) handleGetWorkspaceRuns(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	if c.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "no workspace selected")
		return
	}

	var runs []models.Run
	// Verify workspace belongs to the user to prevent unauthorized access
	var ws models.Workspace
	if err := h.db.First(&ws, "id = ? AND user_id = ?", c.WorkspaceID, c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "workspace not found or access denied")
		return
	}

	if err := h.db.Raw(`
		SELECT r.*, qs.name as question_set_name
		FROM runs r
		JOIN question_sets qs ON r.question_set_id = qs.id
		WHERE r.workspace_id = ?
		ORDER BY r.created_at DESC
	`, c.WorkspaceID).Scan(&runs).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to load runs: "+err.Error())
		return
	}

	c.SendResponse(DataWorkspaceRuns, env.CorrelationID, runs)
}

func (h *Hub) handleGetRunLite(c *Connection, env models.Envelope) {
	var payload models.GetRunLitePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	runID, err := uuid.Parse(payload.RunID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid run_id")
		return
	}

	// Fetch Run
	var run models.Run
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Authorize: only the question set's owner or an accepted collaborator may
	// read this run (prevents cross-tenant IDOR via guessed run IDs).
	if access, _, _, aerr := h.getQuestionSetAccess(h.db, c.UserID, run.QuestionSetID); aerr != nil || !canReadQuestionSet(access) {
		c.SendError(env.CorrelationID, "access denied")
		return
	}

	// Fetch QuestionSet
	var qs models.QuestionSet
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", run.QuestionSetID)
	h.enrichQuestionSetSharing(h.db, &qs, c.UserID)

	// Fetch Results (Lite: Select specific columns only)
	// We need 'answer' to compute hash, but we won't send it.
	// GORM: defining a temporary struct for DB scanning is cleaner.
	type ResultScan struct {
		ID         uuid.UUID
		RunID      uuid.UUID
		AgentID    uuid.UUID
		QuestionID string
		Status     string
		DurationMs int
		CreatedAt  time.Time
		Answer     string
	}
	var scanned []ResultScan

	err = h.db.Model(&models.RunResult{}).
		Select("id, run_id, agent_id, question_id, status, duration_ms, created_at, answer").
		Where("run_id = ?", runID).
		Order("created_at ASC").
		Order("id ASC").
		Scan(&scanned).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch results")
		return
	}

	// Map Scan -> Lite and compute Hash
	results := make([]models.RunResultLite, len(scanned))
	for i, s := range scanned {
		hash := ""
		if s.Answer != "" {
			hObj := sha256.New()
			hObj.Write([]byte(s.Answer))
			hash = hex.EncodeToString(hObj.Sum(nil))
		}

		results[i] = models.RunResultLite{
			ID:          s.ID,
			RunID:       s.RunID,
			AgentID:     s.AgentID,
			QuestionID:  s.QuestionID,
			Status:      s.Status,
			ContentHash: hash,
			DurationMs:  s.DurationMs,
			CreatedAt:   s.CreatedAt,
		}
	}
	results = orchestrator.CollapseRunResultLitesToLatest(results)
	injectEvaluatorTargetIDsLite(results)

	// Fetch Evaluations existence (to set HasEvaluations flag)
	// Optimize: Get all result IDs that have evaluations
	var resultIDsWithEvals []uuid.UUID
	h.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON evaluations.run_result_id = run_results.id").
		Where("run_results.run_id = ?", runID).
		Distinct("run_results.id").
		Pluck("run_results.id", &resultIDsWithEvals)

	// Create a map for quick lookup
	evalsMap := make(map[uuid.UUID]bool)
	for _, id := range resultIDsWithEvals {
		evalsMap[id] = true
	}

	// Update results with HasEvaluations flag
	for i := range results {
		if evalsMap[results[i].ID] {
			results[i].HasEvaluations = true
		}
	}

	// Fetch Agents (Snapshot or Current)
	// Ideally we should store agent snapshot in Run, but for now we look up current agents
	// or try to reconstruct. Here we just fetch all agents referenced in the run results.
	agentIDs := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]bool)
	for _, res := range results {
		if !seen[res.AgentID] {
			agentIDs = append(agentIDs, res.AgentID)
			seen[res.AgentID] = true
		}
	}

	agents := make(map[string]models.Agent)
	if len(agentIDs) > 0 {
		var agentList []models.Agent
		h.db.Find(&agentList, agentIDs)
		for _, a := range agentList {
			agents[a.ID.String()] = a
		}
	}

	resp := models.RunLiteResponse{
		Run:         run,
		QuestionSet: qs,
		Results:     results,
		Agents:      agents,
	}

	c.SendResponse(DataRunLite, env.CorrelationID, resp)
}

func (h *Hub) handleGetLatestRunByQuestionSet(c *Connection, env models.Envelope) {
	var payload models.GetLatestRunByQSPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	// Determine which workspace owns this QS's runs.
	// For shared QSs the runs live in the owner's workspace, not the
	// collaborator's own workspace. getQuestionSetAccess already loads the
	// owning Workspace in a single round-trip and handles owner/editor/viewer
	// uniformly — we use it instead of a bespoke raw SQL scan to avoid
	// UUID/text-column scan errors observed in prod.
	access, _, ownerWs, accessErr := h.getQuestionSetAccess(h.db, c.UserID, qsID)
	if accessErr != nil {
		logger.Warn("[WS] GetLatestRunByQS: access lookup failed user=%s qs=%s err=%v",
			c.UserID, qsID, accessErr)
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	// Fallback: when the connection's workspace already matches the owning
	// workspace, treat it as owner. This covers (a) legacy/test fixtures that
	// don't populate Connection.UserID, and (b) any transient drift where the
	// workspace.user_id lookup happens to miss. It's safe because the WS
	// token is scoped to exactly one workspace.
	if access == accessNone && ownerWs.ID != uuid.Nil && ownerWs.ID == c.WorkspaceID {
		access = accessOwner
	}
	if !canReadQuestionSet(access) {
		logger.Warn("[WS] GetLatestRunByQS: rejected user=%s qs=%s (no access) connWs=%s ownerWs=%s",
			c.UserID, qsID, c.WorkspaceID, ownerWs.ID)
		c.SendError(env.CorrelationID, "not authorized")
		return
	}
	runWorkspaceID := ownerWs.ID

	var run models.Run
	scope := h.db.Where("workspace_id = ? AND question_set_id = ?", runWorkspaceID, qsID)
	if !payload.IncludeRunning {
		scope = scope.Where("status != ?", "running")
	}
	runQuery := scope.
		Order("created_at desc").
		Limit(1).
		Find(&run)
	if err := runQuery.Error; err != nil {
		c.SendError(env.CorrelationID, "failed to fetch run")
		return
	}
	if runQuery.RowsAffected == 0 {
		logger.Debug("[WS] GetLatestRunByQS: no run found user=%s qs=%s access=%d runWs=%s",
			c.UserID, qsID, access, runWorkspaceID)
		c.SendResponse(DataRunLite, env.CorrelationID, map[string]any{
			"run":          nil,
			"question_set": nil,
			"results":      []models.RunResultLite{},
			"agents":       map[string]models.Agent{},
		})
		return
	}

	var qs models.QuestionSet
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", run.QuestionSetID)
	h.enrichQuestionSetSharing(h.db, &qs, c.UserID)

	type ResultScan struct {
		ID         uuid.UUID
		RunID      uuid.UUID
		AgentID    uuid.UUID
		QuestionID string
		Status     string
		DurationMs int
		CreatedAt  time.Time
		Answer     string
		Error      string
	}
	var scanned []ResultScan

	err = h.db.Model(&models.RunResult{}).
		Select("id, run_id, agent_id, question_id, status, duration_ms, created_at, answer, error").
		Where("run_id = ?", run.ID).
		Order("created_at ASC").
		Order("id ASC").
		Scan(&scanned).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch results")
		return
	}

	results := make([]models.RunResultLite, len(scanned))
	for i, s := range scanned {
		hash := ""
		if s.Answer != "" {
			hObj := sha256.New()
			hObj.Write([]byte(s.Answer))
			hash = hex.EncodeToString(hObj.Sum(nil))
		}

		results[i] = models.RunResultLite{
			ID:          s.ID,
			RunID:       s.RunID,
			AgentID:     s.AgentID,
			QuestionID:  s.QuestionID,
			Status:      s.Status,
			ContentHash: hash,
			Error:       s.Error,
			DurationMs:  s.DurationMs,
			CreatedAt:   s.CreatedAt,
		}
	}
	results = orchestrator.CollapseRunResultLitesToLatest(results)
	injectEvaluatorTargetIDsLite(results)

	var resultIDsWithEvals []uuid.UUID
	h.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON evaluations.run_result_id = run_results.id").
		Where("run_results.run_id = ?", run.ID).
		Distinct("run_results.id").
		Pluck("run_results.id", &resultIDsWithEvals)

	evalsMap := make(map[uuid.UUID]bool)
	for _, id := range resultIDsWithEvals {
		evalsMap[id] = true
	}

	for i := range results {
		if evalsMap[results[i].ID] {
			results[i].HasEvaluations = true
		}
	}

	agentIDs := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]bool)
	for _, res := range results {
		if !seen[res.AgentID] {
			agentIDs = append(agentIDs, res.AgentID)
			seen[res.AgentID] = true
		}
	}

	agents := make(map[string]models.Agent)
	if len(agentIDs) > 0 {
		var agentList []models.Agent
		h.db.Find(&agentList, agentIDs)
		for _, a := range agentList {
			agents[a.ID.String()] = a
		}
	}

	resp := models.RunLiteResponse{
		Run:         run,
		QuestionSet: qs,
		Results:     results,
		Agents:      agents,
	}

	c.SendResponse(DataRunLite, env.CorrelationID, resp)
}

func (h *Hub) handleGetResultDetails(c *Connection, env models.Envelope) {
	var payload models.GetResultDetailsPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if len(payload.ResultIDs) == 0 {
		c.SendResponse(DataResultDetails, env.CorrelationID, models.ResultDetailsResponse{Results: []models.RunResult{}})
		return
	}

	var results []models.RunResult
	err := h.db.Preload("Evaluations").
		Where("id IN ?", payload.ResultIDs).
		Find(&results).Error

	if err != nil {
		c.SendError(env.CorrelationID, "failed to fetch details")
		return
	}

	// Authorize: keep only results whose run the caller may read (owner or
	// accepted collaborator of the run's question set). Prevents reading other
	// tenants' answers by guessing result IDs.
	results = h.filterAuthorizedResults(c.UserID, results)

	normalizeResultsEvaluationsForDisplay(results)

	c.SendResponse(DataResultDetails, env.CorrelationID, models.ResultDetailsResponse{Results: results})
}

// filterAuthorizedResults returns only the results whose run the user may read
// (owner or accepted collaborator of the run's question set). Results are
// grouped by run so access is resolved once per run.
func (h *Hub) filterAuthorizedResults(userID uuid.UUID, results []models.RunResult) []models.RunResult {
	runAllowed := make(map[uuid.UUID]bool)
	authorized := results[:0]
	for _, r := range results {
		allowed, known := runAllowed[r.RunID]
		if !known {
			var run models.Run
			if err := h.db.Select("id", "question_set_id").First(&run, "id = ?", r.RunID).Error; err == nil {
				access, _, _, aerr := h.getQuestionSetAccess(h.db, userID, run.QuestionSetID)
				allowed = aerr == nil && canReadQuestionSet(access)
			}
			runAllowed[r.RunID] = allowed
		}
		if allowed {
			authorized = append(authorized, r)
		}
	}
	return authorized
}

func (h *Hub) handleGetRetryStatus(c *Connection, env models.Envelope) {
	var payload models.GetRetryStatusPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if len(payload.RetryIDs) == 0 {
		c.SendResponse(DataRetryStatus, env.CorrelationID, models.RetryStatusResponse{Items: []models.RetryStatusItem{}})
		return
	}

	items := h.engine.GetRetryStatus(c.WorkspaceID, payload.RetryIDs)
	c.SendResponse(DataRetryStatus, env.CorrelationID, models.RetryStatusResponse{Items: items})
}

func (h *Hub) handleDeleteRun(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		RunID string `json:"run_id"`
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

	var run models.Run
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		c.SendError(env.CorrelationID, "run not found")
		return
	}

	// Verify ownership
	if run.WorkspaceID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete Evaluations
		if err := tx.Exec(`
			DELETE FROM evaluations 
			WHERE run_result_id IN (SELECT id FROM run_results WHERE run_id = ?)
		`, runID).Error; err != nil {
			return err
		}

		// 2. Delete RunResults
		if err := tx.Where("run_id = ?", runID).Delete(&models.RunResult{}).Error; err != nil {
			return err
		}

		// 3. Delete Run
		if err := tx.Delete(&run).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete run", err.Error())
		return
	}

	// Notify the full question-set audience (owner + active collaborators)
	// so every connected side drops the run from its list, not just the
	// owner's workspace. Fall back to workspace broadcast if QS fan-out
	// fails for any reason. Drop the run→QS cache entry afterwards.
	deletedPayload := models.DataChangedPayload{
		Resource: "runs",
		Action:   "deleted",
		Data:     map[string]string{"id": runID.String()},
	}
	if run.QuestionSetID != uuid.Nil {
		if err := h.SendEventToQS(run.QuestionSetID, EvtDataChanged, "", deletedPayload); err != nil {
			logger.Warn("[WS] handleDeleteRun: SendEventToQS failed, fallback to workspace: %v", err)
			h.BroadcastEvent(run.WorkspaceID, "runs", "deleted", map[string]string{"id": runID.String()})
		}
	} else {
		h.BroadcastEvent(run.WorkspaceID, "runs", "deleted", map[string]string{"id": runID.String()})
	}
	h.InvalidateRunQSCache(runID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}

func (h *Hub) handleDeleteAllRuns(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	if c.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "no workspace selected")
		return
	}

	// Snapshot (runID, questionSetID) pairs before deletion so we can notify
	// the audience of each involved question set afterwards. Collaborators
	// live outside the owner's workspace and wouldn't otherwise get the
	// "deleted" event from a pure workspace broadcast.
	type runQSPair struct {
		ID            uuid.UUID `gorm:"column:id"`
		QuestionSetID uuid.UUID `gorm:"column:question_set_id"`
	}
	var pairs []runQSPair
	if err := h.db.Raw(`SELECT id, question_set_id FROM runs WHERE workspace_id = ?`, c.WorkspaceID).Scan(&pairs).Error; err != nil {
		logger.Warn("[WS] handleDeleteAllRuns: failed to snapshot runs for broadcast: %v", err)
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete Evaluations for all runs in workspace
		if err := tx.Exec(`
			DELETE FROM evaluations 
			WHERE run_result_id IN (
				SELECT rr.id FROM run_results rr
				JOIN runs r ON rr.run_id = r.id
				WHERE r.workspace_id = ?
			)
		`, c.WorkspaceID).Error; err != nil {
			return err
		}

		// 2. Delete RunResults
		if err := tx.Exec(`
			DELETE FROM run_results 
			WHERE run_id IN (SELECT id FROM runs WHERE workspace_id = ?)
		`, c.WorkspaceID).Error; err != nil {
			return err
		}

		// 3. Delete Runs
		if err := tx.Where("workspace_id = ?", c.WorkspaceID).Delete(&models.Run{}).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete history", err.Error())
		return
	}

	// Always notify the owner's workspace (legacy clients, non-shared QSs).
	h.BroadcastEvent(c.WorkspaceID, "runs", "all_deleted", nil)

	// Also fan out one "deleted" event per run to each shared QS audience so
	// collaborators drop the runs from their UI without a refresh. We reuse
	// the same shape as single-run deletion so the existing frontend handler
	// in wsStore.js ("runs" + "deleted") handles it without changes.
	for _, p := range pairs {
		if p.QuestionSetID == uuid.Nil {
			continue
		}
		if err := h.SendEventToQS(p.QuestionSetID, EvtDataChanged, "", models.DataChangedPayload{
			Resource: "runs",
			Action:   "deleted",
			Data:     map[string]string{"id": p.ID.String()},
		}); err != nil {
			logger.Warn("[WS] handleDeleteAllRuns: SendEventToQS(%s) failed: %v", p.QuestionSetID, err)
		}
		h.InvalidateRunQSCache(p.ID)
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}
