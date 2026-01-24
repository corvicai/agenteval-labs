package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Organization struct {
	ID               uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	Name             string             `gorm:"unique;not null" json:"name"`
	IsSuspended      bool               `gorm:"default:false" json:"is_suspended"`
	AuditLogsEnabled bool               `gorm:"default:false" json:"audit_logs_enabled"`
	ManagerID        *uuid.UUID         `gorm:"type:uuid" json:"manager_id"`
	Manager          *User              `gorm:"foreignKey:ManagerID;constraint:false" json:"manager,omitempty"`
	CreatedByUserID  *uuid.UUID         `gorm:"type:uuid" json:"created_by_user_id"`
	CreatedBy        *User              `gorm:"foreignKey:CreatedByUserID;constraint:false" json:"created_by,omitempty"`
	Users            []User             `gorm:"many2many:user_organizations;" json:"users,omitempty"`
	UserOrgs         []UserOrganization `json:"user_orgs,omitempty"` // For role info
	Workspaces       []Workspace        `json:"workspaces,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
}

type UserOrganization struct {
	UserID         uuid.UUID    `gorm:"type:uuid;primaryKey" json:"user_id"`
	OrganizationID uuid.UUID    `gorm:"type:uuid;primaryKey" json:"organization_id"`
	Organization   Organization `json:"organization,omitempty"`
	Role           string       `gorm:"not null;default:'member'" json:"role"` // 'member', 'manager'
	JoinedAt       time.Time    `json:"joined_at"`
}

type User struct {
	ID              uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	Name            string             `gorm:"not null" json:"name"`
	Email           string             `gorm:"unique;not null" json:"email"`
	PasswordHash    string             `gorm:"not null" json:"-"` // Hide password hash
	IsAdmin         bool               `gorm:"default:false" json:"is_admin"`
	IsSuspended     bool               `gorm:"default:false" json:"is_suspended"`
	Organizations   []Organization     `gorm:"many2many:user_organizations;" json:"organizations,omitempty"`
	UserOrgs        []UserOrganization `json:"user_orgs,omitempty"`
	Workspaces      []Workspace        `json:"workspaces,omitempty"`
	Passkeys        []Passkey          `json:"passkeys,omitempty"`
	InvitedByUserID *uuid.UUID         `gorm:"type:uuid" json:"invited_by_user_id"`
	InvitedBy       *User              `gorm:"foreignKey:InvitedByUserID;constraint:false" json:"invited_by,omitempty"`
	LastLoginAt     *time.Time         `json:"last_login_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
}

type Passkey struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	CredentialID   []byte    `gorm:"unique;not null" json:"credential_id"`
	PublicKey      []byte    `gorm:"not null" json:"public_key"`
	Attestation    string    `json:"attestation"`
	SignCount      uint32    `json:"sign_count"`
	BackupEligible bool      `json:"backup_eligible"`
	BackupState    bool      `json:"backup_state"`
	CreatedAt      time.Time `json:"created_at"`
}

type Workspace struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         uuid.UUID    `gorm:"type:uuid;not null" json:"user_id"`
	User           User         `json:"user"`
	OrganizationID uuid.UUID    `gorm:"type:uuid" json:"organization_id"`
	Organization   Organization `json:"organization"`
	Name           string       `gorm:"not null" json:"name"`
	Clients        []Client     `json:"clients,omitempty"`
	Agents         []Agent      `json:"agents,omitempty"`
	Runs           []Run        `json:"runs,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

type Client struct {
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID  uuid.UUID     `gorm:"type:uuid;not null" json:"workspace_id"`
	Name         string        `gorm:"not null" json:"name"`
	QuestionSets []QuestionSet `json:"question_sets,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

type Agent struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	Name           string         `gorm:"not null" json:"name"`
	ProviderType   string         `gorm:"not null" json:"provider_type"` // 'mcp', 'openai', 'evaluator'
	Config         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"config"`
	Enabled        bool           `gorm:"default:true" json:"enabled"`
	Position       int            `gorm:"default:0" json:"position"`
	MaxConcurrency int            `gorm:"default:5" json:"max_concurrency"` // Max parallel requests (default: 5)
	CreatedAt      time.Time      `json:"created_at"`
}

type QuestionSet struct {
	ID        uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	ClientID  uuid.UUID          `gorm:"type:uuid;not null" json:"client_id"`
	Client    Client             `json:"client,omitempty"`
	Name      string             `gorm:"not null" json:"name"`
	Version   string             `json:"version"`
	Data      datatypes.JSON     `gorm:"type:jsonb;not null" json:"data"`
	Agents    []QuestionSetAgent `gorm:"foreignKey:QuestionSetID" json:"agents,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

type Run struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID     uuid.UUID    `gorm:"type:uuid;not null" json:"workspace_id"`
	QuestionSetID   uuid.UUID    `gorm:"type:uuid;not null" json:"question_set_id"`
	QuestionSet     *QuestionSet `gorm:"foreignKey:QuestionSetID" json:"question_set,omitempty"`
	QuestionSetName string       `gorm:"->;type:text" json:"question_set_name"` // Virtual field for history list
	Status          string       `gorm:"not null;default:'running'" json:"status"`
	TotalTasks      int          `gorm:"default:0" json:"total_tasks"`
	Results         []RunResult  `json:"results,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
}

type RunResult struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RunID       uuid.UUID      `gorm:"type:uuid;not null" json:"run_id"`
	AgentID     uuid.UUID      `gorm:"type:uuid;not null" json:"agent_id"`
	QuestionID  string         `gorm:"not null" json:"question_id"`
	Status      string         `gorm:"not null" json:"status"` // 'success', 'error'
	Answer      string         `json:"answer"`
	Error       string         `json:"error"`
	Metadata    datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	DurationMs  int            `json:"duration_ms"`
	Evaluations []Evaluation   `json:"evaluations,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

type Evaluation struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	RunResultID uuid.UUID `gorm:"type:uuid;not null" json:"run_result_id"`
	RaterType   string    `gorm:"not null" json:"rater_type"` // 'user', 'agent'
	RaterID     uuid.UUID `gorm:"type:uuid" json:"rater_id"`
	Rating      string    `gorm:"not null" json:"rating"` // 'like', 'dislike', 'valid', 'wrong'
	RatingCode  *int      `json:"rating_code"`            // 1=like, 2=valid, 3=dislike, 4=wrong
	Score       *int      `json:"score"`                  // Optional numerical score
	Comments    string    `json:"comments"`
	CreatedAt   time.Time `json:"created_at"`
}

// StatsCache stores pre-computed statistics for on-demand aggregation
type StatsCache struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Scope      string         `gorm:"not null" json:"scope"`     // 'workspace', 'organization', 'global'
	ScopeID    *uuid.UUID     `gorm:"type:uuid" json:"scope_id"` // workspace_id or organization_id (NULL for global)
	Data       datatypes.JSON `gorm:"type:jsonb;not null" json:"data"`
	ComputedAt time.Time      `gorm:"not null" json:"computed_at"`
	ExpiresAt  time.Time      `gorm:"not null" json:"expires_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// StatsData represents the structure of cached statistics
type StatsData struct {
	TotalRuns     int          `json:"total_runs"`
	TotalResults  int          `json:"total_results"`
	SuccessRate   float64      `json:"success_rate"`
	AvgDurationMs float64      `json:"avg_duration_ms"`
	Agents        []AgentStats `json:"agents"`
}

type AgentStats struct {
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	Owner         string    `json:"owner"`
	Count         int       `json:"count"`
	SuccessRate   float64   `json:"success_rate"`
	AvgDurationMs float64   `json:"avg_duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

// QuestionSetAgent represents the many-to-many relationship between Question Sets and Agents
// Each Question Set can have its own agent configuration
type QuestionSetAgent struct {
	QuestionSetID uuid.UUID      `gorm:"type:uuid;primaryKey" json:"question_set_id"`
	AgentID       uuid.UUID      `gorm:"type:uuid;primaryKey" json:"agent_id"`
	Agent         Agent          `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	Config        datatypes.JSON `gorm:"type:jsonb" json:"config,omitempty"`
	Enabled       bool           `json:"enabled"`
	Position      int            `json:"position"`
	CreatedAt     time.Time      `json:"created_at"`
}
type AuditLog struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID    `gorm:"type:uuid;not null;index" json:"organization_id"`
	Organization   Organization `json:"organization"`
	UserID         uuid.UUID    `gorm:"type:uuid;not null" json:"user_id"`
	User           User         `json:"user"`
	Action         string       `gorm:"not null" json:"action"`        // e.g. "CREATE_USER", "SUSPEND_USER"
	ResourceType   string       `gorm:"not null" json:"resource_type"` // e.g. "USER", "WORKSPACE"
	ResourceID     string       `json:"resource_id"`
	Details        string       `gorm:"type:text" json:"details"` // JSON string or description
	RemoteIP       string       `json:"remote_ip"`
	CreatedAt      time.Time    `json:"created_at"`
}
