package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope represents the standard message format
//
// EventID is set by the server on broadcast events so clients can track the
// last event they observed and request missed events (REQ_GET_MISSED_EVENTS)
// after a transient reconnect. It is omitted on request/response envelopes.
type Envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
	EventID       string          `json:"event_id,omitempty"`
}

// GetMissedEventsPayload is the request payload for REQ_GET_MISSED_EVENTS.
type GetMissedEventsPayload struct {
	SinceEventID string `json:"since_event_id"`
}

// MissedEventsResponse is the DATA_MISSED_EVENTS response payload.
//
// When NeedsFullSync is true, the server cannot resume from the given event
// ID (server restart, unknown nonce, or the event rotated out of the buffer)
// and the client must fall back to REQ_SYNC_STATE. Otherwise Events contains
// the ordered envelopes the client missed, already audience-filtered.
type MissedEventsResponse struct {
	NeedsFullSync bool              `json:"needs_full_sync"`
	Events        []json.RawMessage `json:"events,omitempty"`
	LastEventID   string            `json:"last_event_id,omitempty"`
}

// StartRunPayload represents the payload for CMD_START_RUN
type StartRunPayload struct {
	QuestionSetID string   `json:"question_set_id"`
	AgentIDs      []string `json:"agent_ids,omitempty"`
}

type GetQuestionSetAgentEnvelopePayload struct {
	QuestionSetID string `json:"question_set_id"`
}

type QuestionSetAgentEnvelopeResponse struct {
	QuestionSetID   string  `json:"question_set_id"`
	SelectedAgents  []Agent `json:"selected_agents"`
	AvailableAgents []Agent `json:"available_agents"`
}

// RerunTaskPayload represents the payload for CMD_RERUN_TASK
type RerunTaskPayload struct {
	RunID            string `json:"run_id"`
	AgentID          string `json:"agent_id"`
	QuestionID       string `json:"question_id"`
	QuestionSetID    string `json:"question_set_id,omitempty"`
	ResultID         string `json:"result_id,omitempty"`
	OriginalQuestion string `json:"original_question,omitempty"`
	ExpectedAnswer   string `json:"expected_answer,omitempty"`
}

// CancelRunPayload represents the payload for CMD_CANCEL_RUN
type CancelRunPayload struct {
	RunID string `json:"run_id"`
}

// SeedHistoricalRunPayload represents a complete backdated run for seeding
type SeedHistoricalRunPayload struct {
	WorkspaceID   string              `json:"workspace_id,omitempty"`
	QuestionSetID string              `json:"question_set_id"`
	AgentIDs      []string            `json:"agent_ids"`
	CreatedAt     time.Time           `json:"created_at"`
	Results       []SeedResultPayload `json:"results"`
}

type SeedResultPayload struct {
	AgentID     string      `json:"agent_id"`
	QuestionID  string      `json:"question_id"`
	Status      string      `json:"status"` // success, error
	Answer      string      `json:"answer"`
	DurationMs  int         `json:"duration_ms"`
	Evaluations []SeedEvalP `json:"evaluations,omitempty"`
}

type SeedEvalP struct {
	RaterType  string `json:"rater_type"` // user, agent
	Rating     string `json:"rating"`     // like, dislike
	RatingCode *int   `json:"rating_code,omitempty"`
	Score      *int   `json:"score,omitempty"`
	Comments   string `json:"comments"`
}

// TaskStartedPayload represents the payload for EVT_TASK_STARTED
type TaskStartedPayload struct {
	RunID      string `json:"run_id"`
	AgentID    string `json:"agent_id"`
	QuestionID string `json:"question_id"`
}

// TaskCompletedPayload represents the payload for EVT_TASK_COMPLETED
type TaskCompletedPayload struct {
	RunID      string `json:"run_id"`
	AgentID    string `json:"agent_id"`
	QuestionID string `json:"question_id"`
	Success    bool   `json:"success"`
	Answer     string `json:"answer,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int    `json:"duration_ms"`
}

// ManagerStatsPayload response payload
type ManagerStatsPayload struct {
	UserCount      int64 `json:"user_count"`
	WorkspaceCount int64 `json:"workspace_count"`
	AgentCount     int64 `json:"agent_count"`
	RunCount       int64 `json:"run_count"`
}

type ManagerUpdateUserPayload struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type ManagerCreateUserPayload struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ManagerImpersonatePayload struct {
	UserID string `json:"user_id"`
}

// UserResponse similar to handlers.UserResponse but for WS
type UserResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	IsAdmin        bool      `json:"is_admin"`
	IsSuspended    bool      `json:"is_suspended"`
	WorkspaceCount int64     `json:"workspace_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// ActiveRunHydration carries the run_results of the most recent active run in
// the workspace so the frontend can restore in-progress state after reconnect
// without requiring a separate full REQ_GET_RUN_DETAILS call.
type ActiveRunHydration struct {
	RunID         uuid.UUID   `json:"run_id"`
	TotalExpected int         `json:"total_expected"`
	Results       []RunResult `json:"results"`
}

// SharedQuestionSet is a QuestionSet owned by another user that the current
// user has been granted collaboration access to.
type SharedQuestionSet struct {
	QuestionSet
	OwnerUserID      uuid.UUID `json:"owner_user_id"`
	OwnerName        string    `json:"owner_name"`
	OwnerWorkspaceID uuid.UUID `json:"owner_workspace_id"`
	Role             string    `json:"role"`
	AcceptedAt       time.Time `json:"accepted_at"`
	// OwnerAgents are the owner's workspace agents sent with sensitive fields
	// redacted. Collaborators use these to select agents when starting a run.
	OwnerAgents []Agent `json:"owner_agents,omitempty"`
}

// SharedAgent is an Agent owned by another user that the current user has
// been granted use-only access to (Plano 28). Config is always redacted —
// secrets never leave the backend.
type SharedAgent struct {
	Agent
	OwnerUserID uuid.UUID `json:"owner_user_id"`
	OwnerName   string    `json:"owner_name"`
	AcceptedAt  time.Time `json:"accepted_at"`
}

// SyncStatePayload response payload
type SyncStatePayload struct {
	Agents             []Agent             `json:"agents"`
	SharedAgents       []SharedAgent       `json:"shared_agents,omitempty"`
	QuestionSets       []QuestionSet       `json:"question_sets"`
	SharedQuestionSets []SharedQuestionSet `json:"shared_question_sets,omitempty"`
	RecentRuns         []Run               `json:"recent_runs"`
	ActiveRunHydration *ActiveRunHydration `json:"active_run_hydration,omitempty"`
	Warnings           []string            `json:"warnings,omitempty"`
	// EncryptionHealth is only populated for admin users. Contains the current
	// key health so the admin panel can surface a banner without waiting for
	// the operator to navigate to the debug tab.
	EncryptionHealth *AdminDebugKeyStatus `json:"encryption_health,omitempty"`
}

type AdminFilterPayload struct {
	TimeRange string `json:"time_range"`
	Page      int    `json:"page"`       // 0-based page index
	PageSize  int    `json:"page_size"`  // records per page; 0 → default 50, max 100
}

type AdminProfilePayload struct {
	ID string `json:"id"`
}

type AdminRunsPayload struct {
	Limit int `json:"limit"`
}

type AdminRunsSummary struct {
	ActiveRuns       int64 `json:"active_runs"`
	ActiveWorkspaces int64 `json:"active_workspaces"`
	ActiveUsers      int64 `json:"active_users"`
	PendingTasks     int64 `json:"pending_tasks"`
	RecentRuns       int64 `json:"recent_runs"`
}

type AdminRunRecord struct {
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
	PendingCount    int64     `json:"pending_count"`
	ProgressPercent float64   `json:"progress_percent"`
	CreatedAt       time.Time `json:"created_at"`
	LastActivityAt  time.Time `json:"last_activity_at"`
}

type AdminRunsResponse struct {
	Summary     AdminRunsSummary `json:"summary"`
	Runs        []AdminRunRecord `json:"runs"`
	GeneratedAt time.Time        `json:"generated_at"`
}

type AdminDebugRevision struct {
	Commit    string `json:"commit"`
	Branch    string `json:"branch,omitempty"`
	Dirty     string `json:"dirty,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type AdminDebugKeyStatus struct {
	Status                  string     `json:"status,omitempty"`
	Source                  string     `json:"source,omitempty"`
	Summary                 string     `json:"summary,omitempty"`
	Present                 bool       `json:"present"`
	Format                  string     `json:"format,omitempty"`
	CharLength              int        `json:"char_length"`
	ParsedBytes             int        `json:"parsed_bytes"`
	Loaded                  bool       `json:"loaded"`
	UsedFallback            bool       `json:"used_fallback,omitempty"`
	FingerprintPrefix       string     `json:"fingerprint_prefix,omitempty"`
	StatePresent            bool       `json:"state_present"`
	StateStatus             string     `json:"state_status,omitempty"`
	StateSummary            string     `json:"state_summary,omitempty"`
	CipherVersion           string     `json:"cipher_version,omitempty"`
	StoredFingerprintPrefix string     `json:"stored_fingerprint_prefix,omitempty"`
	LastSeenAt              *time.Time `json:"last_seen_at,omitempty"`
	LastMismatchAt          *time.Time `json:"last_mismatch_at,omitempty"`
	Error                   string     `json:"error,omitempty"`
}

type AdminDebugConfigFailure struct {
	ID            string    `json:"id,omitempty"`
	AgentID       string    `json:"agent_id,omitempty"`
	QuestionSetID string    `json:"question_set_id,omitempty"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	Name          string    `json:"name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Shape         string    `json:"shape"`
	Error         string    `json:"error"`
}

type AdminDebugConfigRecord struct {
	ID            string    `json:"id,omitempty"`
	AgentID       string    `json:"agent_id,omitempty"`
	QuestionSetID string    `json:"question_set_id,omitempty"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	Name          string    `json:"name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Shape         string    `json:"shape"`
	DecryptStatus string    `json:"decrypt_status"`
	Error         string    `json:"error,omitempty"`
}

type AdminDebugConfigStats struct {
	Total          int64                     `json:"total"`
	Empty          int64                     `json:"empty"`
	PlaintextJSON  int64                     `json:"plaintext_json"`
	EncryptedLike  int64                     `json:"encrypted_like"`
	InvalidOther   int64                     `json:"invalid_other"`
	DecryptOK      int64                     `json:"decrypt_ok"`
	DecryptFailed  int64                     `json:"decrypt_failed"`
	RecentRecords  []AdminDebugConfigRecord  `json:"recent_records,omitempty"`
	SampleFailures []AdminDebugConfigFailure `json:"sample_failures,omitempty"`
}

type AdminDebugResponse struct {
	AppEnv            string                `json:"app_env"`
	GoVersion         string                `json:"go_version"`
	ServiceName       string                `json:"service_name,omitempty"`
	ServiceRevision   string                `json:"service_revision,omitempty"`
	Revision          AdminDebugRevision    `json:"revision"`
	Key               AdminDebugKeyStatus   `json:"key"`
	Agents            AdminDebugConfigStats `json:"agents"`
	QuestionSetAgents AdminDebugConfigStats `json:"question_set_agents"`
	RecentRunErrors   []AdminDebugRunError  `json:"recent_run_errors,omitempty"`
	GeneratedAt       time.Time             `json:"generated_at"`
}

// AdminDebugRunError is a raw failed-task record surfaced in the admin debug
// snapshot. In environments where admins have no database access (e.g. Cloud
// Run), this is the only window into why runs fail.
type AdminDebugRunError struct {
	RunID        string    `json:"run_id"`
	RunStatus    string    `json:"run_status,omitempty"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
	AgentID      string    `json:"agent_id"`
	AgentName    string    `json:"agent_name,omitempty"`
	ProviderType string    `json:"provider_type,omitempty"`
	QuestionID   string    `json:"question_id"`
	Error        string    `json:"error"`
	DurationMs   int       `json:"duration_ms"`
	CreatedAt    time.Time `json:"created_at"`
}

type AdminCreateUserPayload struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	IsAdmin        bool   `json:"is_admin"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`           // admin, manager, member
	WorkspaceName  string `json:"workspace_name"` // optional
}

type AdminCreateOrgPayload struct {
	Name      string `json:"name"`
	ManagerID string `json:"manager_id,omitempty"`
}

type AdminUpdateUserPayload struct {
	ID             string `json:"id"`
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	IsAdmin        *bool  `json:"is_admin,omitempty"`
	IsSuspended    *bool  `json:"is_suspended,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	Role           string `json:"role,omitempty"`
}

type ChangePasswordPayload struct {
	ID          string `json:"id,omitempty"` // For admins resetting other user's passwords
	OldPassword string `json:"old_password,omitempty"`
	NewPassword string `json:"new_password"`
}

type AdminUpdateOrgPayload struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	ManagerID   string   `json:"manager_id,omitempty"`  // Legacy: single ID
	ManagerIDs  []string `json:"manager_ids,omitempty"` // New: multiple IDs
	IsSuspended *bool    `json:"is_suspended,omitempty"`
}

type AdminRemoveUserFromOrgPayload struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
}

type AdminForceLogoutPayload struct {
	UserID string `json:"user_id,omitempty"` // If empty, logout ALL users
}

type GetRunDetailsPayload struct {
	RunID string `json:"run_id"`
}

type RunDetailsResponse struct {
	Run         Run              `json:"run"`
	QuestionSet QuestionSet      `json:"question_set"`
	Results     []RunResult      `json:"results"`
	Agents      map[string]Agent `json:"agents"`
}

type GetRunLitePayload struct {
	RunID string `json:"run_id"`
}

type GetLatestRunByQSPayload struct {
	QuestionSetID string `json:"question_set_id"`
}

type RunLiteResponse struct {
	Run         Run              `json:"run"`
	QuestionSet QuestionSet      `json:"question_set"`
	Results     []RunResultLite  `json:"results"`
	Agents      map[string]Agent `json:"agents"`
}

type RunResultLite struct {
	ID               uuid.UUID  `json:"id"`
	RunID            uuid.UUID  `json:"run_id"`
	AgentID          uuid.UUID  `json:"agent_id"`
	QuestionID       string     `json:"question_id"`
	Status           string     `json:"status"`
	ContentHash      string     `json:"content_hash"`
	Error            string     `json:"error,omitempty"`
	DurationMs       int        `json:"duration_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	HasEvaluations   bool       `json:"has_evaluations"`
	// TargetRunResultID is set for evaluator results only. It identifies the
	// primary RunResult that this evaluator evaluated so the frontend can detect
	// stale evaluations after a primary-answer retry.
	TargetRunResultID *uuid.UUID `json:"target_run_result_id,omitempty"`
}

type GetResultDetailsPayload struct {
	ResultIDs []string `json:"result_ids"`
}

type ResultDetailsResponse struct {
	Results []RunResult `json:"results"`
}

type GetRetryStatusPayload struct {
	RetryIDs []string `json:"retry_ids"`
}

type RetryStatusItem struct {
	RetryID     string    `json:"retry_id"`
	RunID       string    `json:"run_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	QuestionID  string    `json:"question_id,omitempty"`
	Status      string    `json:"status"`
	RunResultID string    `json:"run_result_id,omitempty"`
	Error       string    `json:"error,omitempty"`
	DurationMs  int       `json:"duration_ms,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RetryStatusResponse struct {
	Items []RetryStatusItem `json:"items"`
}

type RunEvaluatorsPayload struct {
	RunID             string   `json:"run_id"`
	EvaluatorAgentIDs []string `json:"evaluator_agent_ids,omitempty"`
}

type GetSpyPayload struct {
	AgentID  string `json:"agent_id"`
	Question string `json:"question"`
}

type GetWorkspaceStatsPayload struct {
	WorkspaceID string `json:"workspace_id"`
	Force       bool   `json:"force"`
}

type GetOrgStatsPayload struct {
	Force bool `json:"force"`
}

type GetGlobalStatsPayload struct {
	Force bool `json:"force"`
}

// Dev/Auth Payloads
type DevLoginPayload struct {
	UserID string `json:"user_id"`
}

type WsLoginPayload struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	OrganizationID string `json:"organization_id,omitempty"`
}

type ManagerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	OrgName string `json:"org_name"`
}

type WorkspaceWithCount struct {
	Workspace
	AgentCount int64 `json:"agent_count"`
}

type CreateEvaluationPayload struct {
	RunResultID string `json:"run_result_id"`
	Rating      string `json:"rating"`
	RatingCode  *int   `json:"rating_code,omitempty"`
	Score       *int   `json:"score,omitempty"`
	Comments    string `json:"comments"`
}

// DataChangedPayload event payload
type DataChangedPayload struct {
	Resource string `json:"resource"` // "agents", "question_sets", "runs", etc.
	Action   string `json:"action"`   // "created", "updated", "deleted"
	Data     any    `json:"data"`
}

// OnlineStatusPayload represents the payload for EVT_ONLINE_STATUS
type OnlineStatusPayload struct {
	Total   int         `json:"total"`
	UserIDs []uuid.UUID `json:"user_ids"`
}

// WebAuthn Payloads
type WebAuthnRegisterFinishPayload struct {
	Response json.RawMessage `json:"response"`
}

type WebAuthnLoginBeginPayload struct {
	Email string `json:"email"`
}

type WebAuthnLoginFinishPayload struct {
	Email    string          `json:"email"`
	Response json.RawMessage `json:"response"`
}
