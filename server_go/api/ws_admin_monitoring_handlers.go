package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	"benchmarking-platform/internal/buildinfo"
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/security"
	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const adminDebugFailureSampleLimit = 8
const adminDebugRecentRecordLimit = 12
const adminDebugRunErrorLimit = 25
const adminDebugRunErrorMaxLen = 4000

type adminDebugAgentRow struct {
	ID          uuid.UUID `gorm:"column:id"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id"`
	Name        string    `gorm:"column:name"`
	Config      string    `gorm:"column:config"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

type adminDebugQuestionSetAgentRow struct {
	QuestionSetID uuid.UUID `gorm:"column:question_set_id"`
	AgentID       uuid.UUID `gorm:"column:agent_id"`
	Config        string    `gorm:"column:config"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

type adminDebugRunErrorRow struct {
	RunID        uuid.UUID `gorm:"column:run_id"`
	RunStatus    string    `gorm:"column:run_status"`
	WorkspaceID  uuid.UUID `gorm:"column:workspace_id"`
	AgentID      uuid.UUID `gorm:"column:agent_id"`
	AgentName    string    `gorm:"column:agent_name"`
	ProviderType string    `gorm:"column:provider_type"`
	QuestionID   string    `gorm:"column:question_id"`
	Error        string    `gorm:"column:error"`
	DurationMs   int       `gorm:"column:duration_ms"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func buildAdminDebugRunErrors(rows []adminDebugRunErrorRow) []models.AdminDebugRunError {
	out := make([]models.AdminDebugRunError, 0, len(rows))
	for _, row := range rows {
		errText := row.Error
		if len(errText) > adminDebugRunErrorMaxLen {
			errText = errText[:adminDebugRunErrorMaxLen] + "…(truncated)"
		}
		out = append(out, models.AdminDebugRunError{
			RunID:        row.RunID.String(),
			RunStatus:    row.RunStatus,
			WorkspaceID:  row.WorkspaceID.String(),
			AgentID:      row.AgentID.String(),
			AgentName:    row.AgentName,
			ProviderType: row.ProviderType,
			QuestionID:   row.QuestionID,
			Error:        errText,
			DurationMs:   row.DurationMs,
			CreatedAt:    row.CreatedAt,
		})
	}
	return out
}

func (h *Hub) handleAdminGetRuns(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	limit := 100
	var payload models.AdminRunsPayload
	if err := json.Unmarshal([]byte(env.Payload), &payload); err == nil && payload.Limit > 0 {
		limit = payload.Limit
		if limit > 500 {
			limit = 500
		}
	}

	hasCreatedByColumn := h.db.Migrator().HasColumn(&models.Run{}, "created_by_user_id")
	starterJoin := ""
	startedByExpr := "COALESCE(owner.name, 'Unknown')"
	activeUsersExpr := "COUNT(DISTINCT w.user_id)"
	if hasCreatedByColumn {
		starterJoin = "LEFT JOIN users starter ON starter.id = r.created_by_user_id"
		startedByExpr = "COALESCE(starter.name, owner.name, 'Unknown')"
		activeUsersExpr = "COUNT(DISTINCT COALESCE(r.created_by_user_id, w.user_id))"
	}

	type runRow struct {
		ID              uuid.UUID `json:"id"`
		Status          string    `json:"status"`
		WorkspaceID     uuid.UUID `json:"workspace_id"`
		WorkspaceName   string    `json:"workspace_name"`
		QuestionSetName string    `json:"question_set_name"`
		StartedByName   string    `json:"started_by_name"`
		TotalTasks      int       `json:"total_tasks"`
		ResultCount     int64     `json:"result_count"`
		SuccessCount    int64     `json:"success_count"`
		ErrorCount      int64     `json:"error_count"`
		CreatedAt       time.Time `json:"created_at"`
		LastActivityAt  string    `json:"last_activity_at"`
	}

	var runRows []runRow
	runsQuery := fmt.Sprintf(`
		WITH recent_runs AS (
			SELECT
				r.id,
				r.status,
				r.workspace_id,
				r.total_tasks,
				r.created_at,
				w.name AS workspace_name,
				COALESCE(qs.name, '(deleted question set)') AS question_set_name,
				%s AS started_by_name
			FROM runs r
			JOIN workspaces w ON w.id = r.workspace_id
			%s
			LEFT JOIN users owner ON owner.id = w.user_id
			LEFT JOIN question_sets qs ON qs.id = r.question_set_id
			ORDER BY CASE WHEN r.status = 'running' THEN 0 ELSE 1 END, r.created_at DESC
			LIMIT ?
		)
		SELECT
			recent_runs.id,
			recent_runs.status,
			recent_runs.workspace_id,
			recent_runs.workspace_name,
			recent_runs.question_set_name,
			recent_runs.started_by_name,
			recent_runs.total_tasks,
			COUNT(rr.id) AS result_count,
			COALESCE(SUM(CASE WHEN rr.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN rr.status = 'error' THEN 1 ELSE 0 END), 0) AS error_count,
			recent_runs.created_at,
			MAX(rr.created_at) AS last_activity_at
		FROM recent_runs
		LEFT JOIN run_results rr ON rr.run_id = recent_runs.id
		GROUP BY
			recent_runs.id,
			recent_runs.status,
			recent_runs.workspace_id,
			recent_runs.workspace_name,
			recent_runs.question_set_name,
			recent_runs.started_by_name,
			recent_runs.total_tasks,
			recent_runs.created_at
		ORDER BY CASE WHEN recent_runs.status = 'running' THEN 0 ELSE 1 END, recent_runs.created_at DESC
	`, startedByExpr, starterJoin)
	if err := h.db.Raw(runsQuery, limit).Scan(&runRows).Error; err != nil {
		logger.Error("[ADMIN] failed to fetch admin runs: %v", err)
		c.SendError(env.CorrelationID, "failed to fetch runs: "+err.Error())
		return
	}

	var summary models.AdminRunsSummary
	summaryQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) AS active_runs,
			COUNT(DISTINCT r.workspace_id) AS active_workspaces,
			%s AS active_users
		FROM runs r
		JOIN workspaces w ON w.id = r.workspace_id
		WHERE r.status = 'running'
	`, activeUsersExpr)
	if err := h.db.Raw(summaryQuery).Scan(&summary).Error; err != nil {
		logger.Error("[ADMIN] failed to fetch admin run summary: %v", err)
		c.SendError(env.CorrelationID, "failed to fetch run summary: "+err.Error())
		return
	}

	type pendingRow struct {
		TotalTasks  int   `json:"total_tasks"`
		ResultCount int64 `json:"result_count"`
	}
	var pendingRows []pendingRow
	if err := h.db.Raw(`
		SELECT
			r.total_tasks,
			COUNT(rr.id) AS result_count
		FROM runs r
		LEFT JOIN run_results rr ON rr.run_id = r.id
		WHERE r.status = 'running'
		GROUP BY r.id, r.total_tasks
	`).Scan(&pendingRows).Error; err != nil {
		logger.Error("[ADMIN] failed to calculate admin pending tasks: %v", err)
		c.SendError(env.CorrelationID, "failed to calculate pending tasks: "+err.Error())
		return
	}

	var runs []models.AdminRunRecord
	runs = make([]models.AdminRunRecord, 0, len(runRows))
	var totalPendingTasks int64
	for _, row := range runRows {
		pendingCount := int64(row.TotalTasks) - row.ResultCount
		if pendingCount < 0 {
			pendingCount = 0
		}

		progressPercent := 0.0
		if row.TotalTasks > 0 {
			progressPercent = (float64(row.ResultCount) / float64(row.TotalTasks)) * 100
			if progressPercent > 100 {
				progressPercent = 100
			}
		}

		lastActivityAt := parseAdminRunTimestamp(row.LastActivityAt, row.CreatedAt)

		runs = append(runs, models.AdminRunRecord{
			ID:              row.ID,
			Status:          row.Status,
			WorkspaceID:     row.WorkspaceID,
			WorkspaceName:   row.WorkspaceName,
			QuestionSetName: row.QuestionSetName,
			StartedByName:   row.StartedByName,
			TotalTasks:      row.TotalTasks,
			ResultCount:     row.ResultCount,
			SuccessCount:    row.SuccessCount,
			ErrorCount:      row.ErrorCount,
			PendingCount:    pendingCount,
			ProgressPercent: progressPercent,
			CreatedAt:       row.CreatedAt,
			LastActivityAt:  lastActivityAt,
		})
	}

	for _, row := range pendingRows {
		pendingCount := int64(row.TotalTasks) - row.ResultCount
		if pendingCount > 0 {
			totalPendingTasks += pendingCount
		}
	}

	summary.PendingTasks = totalPendingTasks
	summary.RecentRuns = int64(len(runs))

	c.SendResponse(DataAdminRuns, env.CorrelationID, models.AdminRunsResponse{
		Summary:     summary,
		Runs:        runs,
		GeneratedAt: time.Now().UTC(),
	})
}

func (h *Hub) handleAdminGetDebugInfo(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	var agentRows []adminDebugAgentRow
	if err := h.db.Raw(`
		SELECT id, workspace_id, name, COALESCE(config, '') AS config, created_at
		FROM agents
		ORDER BY created_at DESC
	`).Scan(&agentRows).Error; err != nil {
		logger.Error("[ADMIN] failed to inspect agent configs: %v", err)
		c.SendError(env.CorrelationID, "failed to inspect agent configs: "+err.Error())
		return
	}

	var questionSetAgentRows []adminDebugQuestionSetAgentRow
	if err := h.db.Raw(`
		SELECT question_set_id, agent_id, COALESCE(config, '') AS config, created_at
		FROM question_set_agents
		ORDER BY created_at DESC
	`).Scan(&questionSetAgentRows).Error; err != nil {
		logger.Error("[ADMIN] failed to inspect question set agent configs: %v", err)
		c.SendError(env.CorrelationID, "failed to inspect question set agent configs: "+err.Error())
		return
	}

	var runErrorRows []adminDebugRunErrorRow
	if err := h.db.Raw(`
		SELECT rr.run_id AS run_id, r.status AS run_status, r.workspace_id AS workspace_id,
		       rr.agent_id AS agent_id, COALESCE(a.name, '') AS agent_name,
		       COALESCE(a.provider_type, '') AS provider_type,
		       rr.question_id AS question_id, COALESCE(rr.error, '') AS error,
		       rr.duration_ms AS duration_ms, rr.created_at AS created_at
		FROM run_results rr
		LEFT JOIN runs r ON r.id = rr.run_id
		LEFT JOIN agents a ON a.id = rr.agent_id
		WHERE rr.status = 'error'
		ORDER BY rr.created_at DESC
		LIMIT ?
	`, adminDebugRunErrorLimit).Scan(&runErrorRows).Error; err != nil {
		logger.Error("[ADMIN] failed to inspect recent run errors: %v", err)
		c.SendError(env.CorrelationID, "failed to inspect recent run errors: "+err.Error())
		return
	}

	encryptionKeyHealth, err := service.NewEncryptionKeyService(h.db).InspectCurrentKeyHealth()
	if err != nil {
		logger.Warn("[ADMIN] failed to inspect persisted encryption key state: %v", err)
	}

	response := models.AdminDebugResponse{
		AppEnv:            strings.TrimSpace(os.Getenv("APP_ENV")),
		GoVersion:         goruntime.Version(),
		ServiceName:       strings.TrimSpace(os.Getenv("K_SERVICE")),
		ServiceRevision:   strings.TrimSpace(os.Getenv("K_REVISION")),
		Revision:          buildAdminDebugRevision(),
		Key:               buildAdminDebugKeyStatus(encryptionKeyHealth),
		Agents:            analyzeAdminDebugAgents(agentRows),
		QuestionSetAgents: analyzeAdminDebugQuestionSetAgents(questionSetAgentRows),
		RecentRunErrors:   buildAdminDebugRunErrors(runErrorRows),
		GeneratedAt:       time.Now().UTC(),
	}

	if response.AppEnv == "" {
		response.AppEnv = "development"
	}

	c.SendResponse(DataAdminDebugInfo, env.CorrelationID, response)
}

func parseAdminDebugConfig(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "empty", nil
	}

	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		return "plaintext_json", nil
	}

	shape := "invalid_other"
	if _, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
		shape = "encrypted_like"
	}

	_, err := security.Decrypt(trimmed)
	return shape, err
}

func analyzeAdminDebugAgents(rows []adminDebugAgentRow) models.AdminDebugConfigStats {
	stats := models.AdminDebugConfigStats{Total: int64(len(rows))}

	for index, row := range rows {
		shape, err := parseAdminDebugConfig(row.Config)
		decryptStatus := adminDebugDecryptStatus(shape, err)
		switch shape {
		case "empty":
			stats.Empty++
		case "plaintext_json":
			stats.PlaintextJSON++
		case "encrypted_like":
			stats.EncryptedLike++
		default:
			stats.InvalidOther++
		}

		if index < adminDebugRecentRecordLimit {
			record := models.AdminDebugConfigRecord{
				ID:            row.ID.String(),
				WorkspaceID:   row.WorkspaceID.String(),
				Name:          row.Name,
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				DecryptStatus: decryptStatus,
			}
			if err != nil && decryptStatus == "failed" {
				record.Error = err.Error()
			}
			stats.RecentRecords = append(stats.RecentRecords, record)
		}

		if err == nil || shape == "empty" || shape == "plaintext_json" {
			if shape == "encrypted_like" {
				stats.DecryptOK++
			}
			continue
		}

		stats.DecryptFailed++
		if len(stats.SampleFailures) < adminDebugFailureSampleLimit {
			stats.SampleFailures = append(stats.SampleFailures, models.AdminDebugConfigFailure{
				ID:          row.ID.String(),
				WorkspaceID: row.WorkspaceID.String(),
				Name:        row.Name,
				CreatedAt:   row.CreatedAt,
				Shape:       shape,
				Error:       err.Error(),
			})
		}
	}

	return stats
}

func analyzeAdminDebugQuestionSetAgents(rows []adminDebugQuestionSetAgentRow) models.AdminDebugConfigStats {
	stats := models.AdminDebugConfigStats{Total: int64(len(rows))}

	for index, row := range rows {
		shape, err := parseAdminDebugConfig(row.Config)
		decryptStatus := adminDebugDecryptStatus(shape, err)
		switch shape {
		case "empty":
			stats.Empty++
		case "plaintext_json":
			stats.PlaintextJSON++
		case "encrypted_like":
			stats.EncryptedLike++
		default:
			stats.InvalidOther++
		}

		if index < adminDebugRecentRecordLimit {
			record := models.AdminDebugConfigRecord{
				QuestionSetID: row.QuestionSetID.String(),
				AgentID:       row.AgentID.String(),
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				DecryptStatus: decryptStatus,
			}
			if err != nil && decryptStatus == "failed" {
				record.Error = err.Error()
			}
			stats.RecentRecords = append(stats.RecentRecords, record)
		}

		if err == nil || shape == "empty" || shape == "plaintext_json" {
			if shape == "encrypted_like" {
				stats.DecryptOK++
			}
			continue
		}

		stats.DecryptFailed++
		if len(stats.SampleFailures) < adminDebugFailureSampleLimit {
			stats.SampleFailures = append(stats.SampleFailures, models.AdminDebugConfigFailure{
				AgentID:       row.AgentID.String(),
				QuestionSetID: row.QuestionSetID.String(),
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				Error:         err.Error(),
			})
		}
	}

	return stats
}

func adminDebugDecryptStatus(shape string, err error) string {
	if err == nil {
		if shape == "encrypted_like" {
			return "ok"
		}
		return "not_applicable"
	}
	return "failed"
}

func buildAdminDebugRevision() models.AdminDebugRevision {
	return models.AdminDebugRevision{
		Commit:    firstNonEmptyAdminDebug(buildinfo.Commit, os.Getenv("APP_REVISION"), os.Getenv("GIT_COMMIT")),
		Branch:    firstNonEmptyAdminDebug(buildinfo.Branch, os.Getenv("APP_REVISION_BRANCH")),
		Dirty:     firstNonEmptyAdminDebug(buildinfo.Dirty, os.Getenv("APP_REVISION_DIRTY")),
		UpdatedAt: firstNonEmptyAdminDebug(buildinfo.UpdatedAt, os.Getenv("APP_REVISION_UPDATED_AT")),
	}
}

func buildAdminDebugKeyStatus(health service.EncryptionKeyHealth) models.AdminDebugKeyStatus {
	raw := os.Getenv("ENCRYPTION_KEY")
	runtimeStatus := security.GetEncryptionKeyRuntimeStatus()
	status := models.AdminDebugKeyStatus{
		Status:                  runtimeStatus.Status,
		Source:                  runtimeStatus.Source,
		Summary:                 runtimeStatus.Summary,
		Present:                 strings.TrimSpace(raw) != "",
		CharLength:              len(raw),
		Loaded:                  runtimeStatus.Loaded,
		UsedFallback:            runtimeStatus.UsedFallback,
		StatePresent:            health.StatePresent,
		StateStatus:             health.StateStatus,
		StateSummary:            health.StateSummary,
		CipherVersion:           health.CipherVersion,
		FingerprintPrefix:       health.ObservedFingerprintPrefix,
		StoredFingerprintPrefix: health.StoredFingerprintPrefix,
		LastSeenAt:              health.LastSeenAt,
		LastMismatchAt:          health.LastMismatchAt,
	}

	if !status.Present {
		if status.Status == "" {
			status.Status = "missing"
		}
		if status.Summary == "" {
			status.Summary = "ENCRYPTION_KEY is not set"
		}
		status.Error = "ENCRYPTION_KEY environment variable not set"
		return status
	}

	key, format, err := security.ParseEncryptionKey(raw)
	if err != nil {
		if status.Status == "" {
			status.Status = "invalid"
		}
		if status.Source == "" {
			status.Source = "environment"
		}
		if status.Summary == "" {
			status.Summary = "ENCRYPTION_KEY is present but invalid"
		}
		status.Error = err.Error()
		return status
	}

	status.Format = firstNonEmptyAdminDebug(runtimeStatus.Format, format)
	status.ParsedBytes = maxAdminDebugInt(runtimeStatus.ParsedBytes, len(key))
	if status.FingerprintPrefix == "" {
		status.FingerprintPrefix = shortAdminDebugFingerprint(security.KeyFingerprint(key))
	}
	if status.Status == "" {
		status.Status = "loaded"
	}
	if status.Source == "" {
		status.Source = "environment"
	}
	if status.Summary == "" {
		if status.Format == "hex" {
			status.Summary = "ENCRYPTION_KEY loaded successfully from a hex-encoded environment value"
		} else {
			status.Summary = "ENCRYPTION_KEY loaded successfully from environment"
		}
	}
	status.Loaded = true
	return status
}

func firstNonEmptyAdminDebug(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func maxAdminDebugInt(current, fallback int) int {
	if current > 0 {
		return current
	}
	return fallback
}

func shortAdminDebugFingerprint(fingerprint string) string {
	trimmed := strings.TrimSpace(fingerprint)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}

func parseAdminRunTimestamp(raw string, fallback time.Time) time.Time {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return fallback
	}

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, candidate); err == nil {
			return parsed
		}
	}

	return fallback
}

// encryptionKeyHealthForAdmin returns an AdminDebugKeyStatus for use in the
// sync state response sent to admin users. It reuses buildAdminDebugKeyStatus
// (already used by handleAdminDebugInfo) so the data is consistent.
// Returns nil when the health is "match" (nothing to surface) or if the check fails.
func encryptionKeyHealthForAdmin(db *gorm.DB) (*models.AdminDebugKeyStatus, error) {
	health, err := service.NewEncryptionKeyService(db).InspectCurrentKeyHealth()
	if err != nil {
		return nil, err
	}
	status := buildAdminDebugKeyStatus(health)
	// Only include the health payload when there is something to warn about.
	if status.StateStatus == "match" || status.StateStatus == "" {
		return nil, nil
	}
	return &status, nil
}
