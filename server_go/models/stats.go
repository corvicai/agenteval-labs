package models

import (
	"time"

	"github.com/google/uuid"
)

type AgentPerformance struct {
	AgentID       uuid.UUID `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	SuccessRate   float64   `json:"success_rate"`
	AvgDurationMs float64   `json:"avg_duration_ms"`
	Count         int64     `json:"count"`
	Owner         string    `json:"owner"`
	OrgName       string    `json:"org_name"`
	CreatedAt     time.Time `json:"created_at"`

	// Evaluation stats
	TotalEvaluations int64   `json:"total_evaluations"`
	LikesCount       int64   `json:"likes_count"`
	ValidsCount      int64   `json:"valids_count"`
	DislikesCount    int64   `json:"dislikes_count"`
	WrongsCount      int64   `json:"wrongs_count"`
	PositiveRate     float64 `json:"positive_rate"` // (likes + valids) / total_evaluations
	NegativeRate     float64 `json:"negative_rate"` // (dislikes + wrongs) / total_evaluations
	AvgScore         float64 `json:"avg_score"`
}

type AggregatedStats struct {
	TotalRuns     int64              `json:"total_runs"`
	TotalResults  int64              `json:"total_results"`
	SuccessRate   float64            `json:"success_rate"`
	AvgDurationMs float64            `json:"avg_duration_ms"`
	Agents        []AgentPerformance `json:"agents"`
	ComputedAt    time.Time          `json:"computed_at"`
	CacheHit      bool               `json:"cache_hit"`

	// Aggregated Evaluation stats
	TotalEvaluations int64            `json:"total_evaluations"`
	LikesCount       int64            `json:"likes_count"`
	ValidsCount      int64            `json:"valids_count"`
	DislikesCount    int64            `json:"dislikes_count"`
	WrongsCount      int64            `json:"wrongs_count"`
	PositiveRate     float64          `json:"positive_rate"`
	NegativeRate     float64          `json:"negative_rate"`
	AvgScore         float64          `json:"avg_score"`
	History          []StatsDataPoint `json:"history"`
}

type StatsDataPoint struct {
	Date     string  `json:"date"`
	AvgScore float64 `json:"avg_score"`
	Count    int64   `json:"count"`
}
