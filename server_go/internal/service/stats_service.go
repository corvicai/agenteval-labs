package service

import (
	"log"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StatsService struct {
	db *gorm.DB
}

func NewStatsService(db *gorm.DB) *StatsService {
	return &StatsService{db: db}
}

func (s *StatsService) ComputeStats(scope string, scopeID *uuid.UUID) (*models.AggregatedStats, error) {
	var stats models.AggregatedStats
	log.Printf("[Stats] Computing fresh stats for scope: %s, ID: %v", scope, scopeID)

	baseResultQuery := s.db.Model(&models.RunResult{})
	baseRunQuery := s.db.Model(&models.Run{})

	switch scope {
	case "workspace":
		baseResultQuery = baseResultQuery.Joins("JOIN runs ON runs.id = run_results.run_id").
			Where("runs.workspace_id = ?", *scopeID)
		baseRunQuery = baseRunQuery.Where("workspace_id = ?", *scopeID)
	case "organization":
		baseResultQuery = baseResultQuery.Joins("JOIN runs ON runs.id = run_results.run_id").
			Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Joins("JOIN user_organizations ON user_organizations.user_id = workspaces.user_id").
			Where("user_organizations.organization_id = ?", *scopeID)
		baseRunQuery = baseRunQuery.Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Joins("JOIN user_organizations ON user_organizations.user_id = workspaces.user_id").
			Where("user_organizations.organization_id = ?", *scopeID)
	case "global":
		// No filter
	}

	baseRunQuery.Count(&stats.TotalRuns)

	var totalRes int64
	baseResultQuery.Session(&gorm.Session{}).Count(&totalRes)
	stats.TotalResults = totalRes

	if totalRes > 0 {
		var successCount int64
		baseResultQuery.Session(&gorm.Session{}).Where("run_results.status = ?", "success").Count(&successCount)
		stats.SuccessRate = float64(successCount) / float64(totalRes)

		var avgDuration float64
		baseResultQuery.Session(&gorm.Session{}).Select("COALESCE(AVG(run_results.duration_ms), 0)").Row().Scan(&avgDuration)
		stats.AvgDurationMs = avgDuration
	}

	// Computation of Aggregate Evaluation Stats
	var evalSummary struct {
		Total    int64
		Likes    int64
		Valids   int64
		Dislikes int64
		Wrongs   int64
		AvgScore float64
	}

	baseEvalQuery := s.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON run_results.id = evaluations.run_result_id").
		Joins("JOIN runs ON runs.id = run_results.run_id")

	switch scope {
	case "workspace":
		baseEvalQuery = baseEvalQuery.Where("runs.workspace_id = ?", *scopeID)
	case "organization":
		baseEvalQuery = baseEvalQuery.Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Joins("JOIN user_organizations ON user_organizations.user_id = workspaces.user_id").
			Where("user_organizations.organization_id = ?", *scopeID)
	}

	baseEvalQuery.Select(`
		COUNT(*) as total,
		SUM(CASE WHEN rating_code = 1 THEN 1 ELSE 0 END) as likes,
		SUM(CASE WHEN rating_code = 2 THEN 1 ELSE 0 END) as valids,
		SUM(CASE WHEN rating_code = 3 THEN 1 ELSE 0 END) as dislikes,
		SUM(CASE WHEN rating_code = 4 THEN 1 ELSE 0 END) as wrongs,
		COALESCE(AVG(COALESCE(score, CASE
			WHEN rating_code = 1 THEN 100
			WHEN rating_code = 2 THEN 75
			WHEN rating_code = 3 THEN 25
			WHEN rating_code = 4 THEN 0
			ELSE NULL
		END)), 0) as avg_score
	`).Scan(&evalSummary)

	stats.TotalEvaluations = evalSummary.Total
	stats.LikesCount = evalSummary.Likes
	stats.ValidsCount = evalSummary.Valids
	stats.DislikesCount = evalSummary.Dislikes
	stats.WrongsCount = evalSummary.Wrongs
	stats.AvgScore = evalSummary.AvgScore
	if evalSummary.Total > 0 {
		stats.PositiveRate = float64(evalSummary.Likes+evalSummary.Valids) / float64(evalSummary.Total)
		stats.NegativeRate = float64(evalSummary.Dislikes+evalSummary.Wrongs) / float64(evalSummary.Total)
	}

	var agentRows []struct {
		AgentID      uuid.UUID
		AvgDur       float64
		Total        int64
		Success      int64
		EvalTotal    int64
		EvalLikes    int64
		EvalValids   int64
		EvalDislikes int64
		EvalWrongs   int64
		EvalAvgScore float64
	}

	baseResultQuery.Session(&gorm.Session{}).
		Select(`
			run_results.agent_id, 
			AVG(run_results.duration_ms) as avg_dur, 
			COUNT(*) as total, 
			SUM(CASE WHEN run_results.status = 'success' THEN 1 ELSE 0 END) as success,
			COUNT(e.id) as eval_total,
			SUM(CASE WHEN e.rating_code = 1 THEN 1 ELSE 0 END) as eval_likes,
			SUM(CASE WHEN e.rating_code = 2 THEN 1 ELSE 0 END) as eval_valids,
			SUM(CASE WHEN e.rating_code = 3 THEN 1 ELSE 0 END) as eval_dislikes,
			SUM(CASE WHEN e.rating_code = 4 THEN 1 ELSE 0 END) as eval_wrongs,
			COALESCE(AVG(COALESCE(e.score, CASE
				WHEN e.rating_code = 1 THEN 100
				WHEN e.rating_code = 2 THEN 75
				WHEN e.rating_code = 3 THEN 25
				WHEN e.rating_code = 4 THEN 0
				ELSE NULL
			END)), 0) as eval_avg_score
		`).
		Joins("LEFT JOIN evaluations e ON e.run_result_id = run_results.id").
		Group("run_results.agent_id").
		Scan(&agentRows)

	for _, row := range agentRows {
		var agent models.Agent
		if err := s.db.First(&agent, "id = ?", row.AgentID).Error; err != nil {
			continue
		}

		ap := models.AgentPerformance{
			AgentID:          row.AgentID,
			AgentName:        agent.Name,
			AvgDurationMs:    row.AvgDur,
			Count:            row.Total,
			CreatedAt:        agent.CreatedAt,
			TotalEvaluations: row.EvalTotal,
			LikesCount:       row.EvalLikes,
			ValidsCount:      row.EvalValids,
			DislikesCount:    row.EvalDislikes,
			WrongsCount:      row.EvalWrongs,
			AvgScore:         row.EvalAvgScore,
		}
		if row.Total > 0 {
			ap.SuccessRate = float64(row.Success) / float64(row.Total)
		}
		if row.EvalTotal > 0 {
			ap.PositiveRate = float64(row.EvalLikes+row.EvalValids) / float64(row.EvalTotal)
			ap.NegativeRate = float64(row.EvalDislikes+row.EvalWrongs) / float64(row.EvalTotal)
		}

		var workspace models.Workspace
		if s.db.Preload("User").First(&workspace, "id = ?", agent.WorkspaceID).Error == nil {
			if workspace.User.ID != uuid.Nil {
				ap.Owner = workspace.User.Name
			} else {
				ap.Owner = "System"
			}
			ap.OrgName = "" // Organizations removed
		}
		stats.Agents = append(stats.Agents, ap)
	}

	// 5. History (Last 30 days)
	historyQuery := s.db.Table("evaluations").
		Select(`
			TO_CHAR(evaluations.created_at, 'YYYY-MM-DD') as date,
			AVG(COALESCE(score, CASE
				WHEN rating_code = 1 THEN 100
				WHEN rating_code = 2 THEN 75
				WHEN rating_code = 3 THEN 25
				WHEN rating_code = 4 THEN 0
				ELSE NULL
			END)) as avg_score,
			COUNT(*) as count
		`).
		Joins("JOIN run_results ON run_results.id = evaluations.run_result_id").
		Joins("JOIN runs ON runs.id = run_results.run_id")

	switch scope {
	case "workspace":
		historyQuery = historyQuery.Where("runs.workspace_id = ?", *scopeID)
	case "organization":
		historyQuery = historyQuery.Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Joins("JOIN user_organizations ON user_organizations.user_id = workspaces.user_id").
			Where("user_organizations.organization_id = ?", *scopeID)
	}

	historyQuery.Where("evaluations.created_at > ?", time.Now().UTC().AddDate(0, 0, -30)).
		Group("date").
		Order("date ASC").
		Scan(&stats.History)

	stats.ComputedAt = time.Now().UTC()
	stats.CacheHit = false
	return &stats, nil
}
