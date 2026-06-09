package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type RunComparisonTemplate struct {
    ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    WorkspaceID      uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
    CreatedByUserID  *uuid.UUID     `gorm:"type:uuid" json:"created_by_user_id,omitempty"`
    Name             string         `gorm:"not null" json:"name"`
    Config           datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"config"`
    CreatedAt        time.Time      `json:"created_at"`
    UpdatedAt        time.Time      `json:"updated_at"`
}

func (RunComparisonTemplate) TableName() string { return "run_comparison_templates" }

type RunComparison struct {
    ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
    TemplateID        *uuid.UUID     `gorm:"type:uuid" json:"template_id,omitempty"`
    CreatedByUserID   *uuid.UUID     `gorm:"type:uuid" json:"created_by_user_id,omitempty"`
    Name              string         `gorm:"not null" json:"name"`
    RunIDs            datatypes.JSON `gorm:"type:jsonb;not null" json:"run_ids"`
    Labels            datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"labels"`
    MetricsEnabled    datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"metrics_enabled"`
    ReportData        datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"report_data"`
    RunsSnapshotHash  string         `gorm:"not null;default:''" json:"runs_snapshot_hash"`
    CreatedAt         time.Time      `json:"created_at"`

    // Transient — populado ao ler (não persistido):
    IsStale bool `gorm:"-" json:"is_stale,omitempty"`
}

func (RunComparison) TableName() string { return "run_comparisons" }
