package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope represents the standard message format
type Envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

// StartRunPayload represents the payload for CMD_START_RUN
type StartRunPayload struct {
	QuestionSetID string   `json:"question_set_id"`
	AgentIDs      []string `json:"agent_ids,omitempty"`
}

// RerunTaskPayload represents the payload for CMD_RERUN_TASK
type RerunTaskPayload struct {
	RunID      string `json:"run_id"`
	AgentID    string `json:"agent_id"`
	QuestionID string `json:"question_id"`
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

// SyncStatePayload response payload
type SyncStatePayload struct {
	Agents       []Agent       `json:"agents"`
	QuestionSets []QuestionSet `json:"question_sets"`
	RecentRuns   []Run         `json:"recent_runs"`
}

type AdminProfilePayload struct {
	ID string `json:"id"`
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
}

type AdminUpdateOrgPayload struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ManagerID   string `json:"manager_id,omitempty"`
	IsSuspended *bool  `json:"is_suspended,omitempty"`
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
	ID             uuid.UUID `json:"id"`
	RunID          uuid.UUID `json:"run_id"`
	AgentID        uuid.UUID `json:"agent_id"`
	QuestionID     string    `json:"question_id"`
	Status         string    `json:"status"`
	ContentHash    string    `json:"content_hash"`
	DurationMs     int       `json:"duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
	HasEvaluations bool      `json:"has_evaluations"`
}

type GetResultDetailsPayload struct {
	ResultIDs []string `json:"result_ids"`
}

type ResultDetailsResponse struct {
	Results []RunResult `json:"results"`
}

type RunEvaluatorsPayload struct {
	RunID string `json:"run_id"`
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
