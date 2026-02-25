package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"benchmarking-platform/internal/logger"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

// Cache TTL constants
const (
	WorkspaceCacheTTL    = 5 * time.Minute
	OrganizationCacheTTL = 15 * time.Minute
	GlobalCacheTTL       = 30 * time.Minute
)

type StatsHandler struct {
	db *gorm.DB
}

func NewStatsHandler(db *gorm.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

type AggregatedStats struct {
	TotalRuns     int64              `json:"total_runs"`
	TotalResults  int64              `json:"total_results"`
	SuccessRate   float64            `json:"success_rate"`
	AvgDurationMs float64            `json:"avg_duration_ms"`
	Agents        []AgentPerformance `json:"agents"`
	ComputedAt    time.Time          `json:"computed_at"`
	CacheHit      bool               `json:"cache_hit"`

	// Evaluation stats
	TotalEvaluations int64   `json:"total_evaluations"`
	LikesCount       int64   `json:"likes_count"`
	ValidsCount      int64   `json:"valids_count"`
	DislikesCount    int64   `json:"dislikes_count"`
	WrongsCount      int64   `json:"wrongs_count"`
	PositiveRate     float64 `json:"positive_rate"`
	NegativeRate     float64 `json:"negative_rate"`
	AvgScore         float64 `json:"avg_score"`
}

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
	PositiveRate     float64 `json:"positive_rate"`
	NegativeRate     float64 `json:"negative_rate"`
	AvgScore         float64 `json:"avg_score"`
}

// GetWorkspaceStats returns stats for a specific workspace (with caching)
func (h *StatsHandler) GetWorkspaceStats(c echo.Context) error {
	wsIDStr := c.Param("workspace_id")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workspace ID"})
	}

	forceRefresh := c.QueryParam("force") == "true"
	return h.getCachedStats(c, "workspace", &wsID, WorkspaceCacheTTL, forceRefresh)
}

// GetOrganizationStats returns stats for all workspaces in an organization (with caching)
func (h *StatsHandler) GetOrganizationStats(c echo.Context) error {
	orgID := middleware.GetOrgID(c)
	if orgID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Organization context missing"})
	}

	forceRefresh := c.QueryParam("force") == "true"
	return h.getCachedStats(c, "organization", &orgID, OrganizationCacheTTL, forceRefresh)
}

// GetGlobalStats returns stats for all organizations (Super Admin only, with caching)
func (h *StatsHandler) GetGlobalStats(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	if !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Super Admin access required for global stats"})
	}

	forceRefresh := c.QueryParam("force") == "true"
	return h.getCachedStats(c, "global", nil, GlobalCacheTTL, forceRefresh)
}

// RecalculateStats forces recalculation of all cached stats (Admin only)
func (h *StatsHandler) RecalculateStats(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	if !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	// Invalidate all caches by deleting them
	h.db.Where("1=1").Delete(&models.StatsCache{})

	return c.JSON(http.StatusOK, map[string]string{"message": "All stats caches invalidated. Next requests will recompute."})
}

// getCachedStats attempts to retrieve stats from cache, or computes fresh if needed
func (h *StatsHandler) getCachedStats(c echo.Context, scope string, scopeID *uuid.UUID, ttl time.Duration, forceRefresh bool) error {
	// Try to get from cache (unless force refresh)
	if !forceRefresh {
		var cache models.StatsCache
		query := h.db.Where("scope = ?", scope)
		if scopeID != nil {
			query = query.Where("scope_id = ?", *scopeID)
		} else {
			query = query.Where("scope_id IS NULL")
		}

		if err := query.First(&cache).Error; err == nil {
			// Check if not expired
			if time.Now().UTC().Before(cache.ExpiresAt) {
				var stats AggregatedStats
				if err := json.Unmarshal(cache.Data, &stats); err == nil {
					stats.CacheHit = true
					stats.ComputedAt = cache.ComputedAt
					return c.JSON(http.StatusOK, stats)
				}
			}
		}
	}

	// Cache miss or expired - compute fresh stats
	stats, err := h.computeStats(scope, scopeID, c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to compute stats"})
	}

	// Save to cache
	h.saveToCache(scope, scopeID, stats, ttl)

	stats.CacheHit = false
	stats.ComputedAt = time.Now().UTC()
	return c.JSON(http.StatusOK, stats)
}

// computeStats calculates fresh statistics for the given scope
func (h *StatsHandler) computeStats(scope string, scopeID *uuid.UUID, c echo.Context) (*AggregatedStats, error) {
	var stats AggregatedStats

	logger.Info("[Stats] Computing fresh stats for scope: %s, ID: %v", scope, scopeID)

	// Define base query for results based on scope
	baseResultQuery := h.db.Model(&models.RunResult{})
	baseRunQuery := h.db.Model(&models.Run{})

	switch scope {
	case "workspace":
		baseResultQuery = baseResultQuery.Joins("JOIN runs ON runs.id = run_results.run_id").
			Where("runs.workspace_id = ?", *scopeID)
		baseRunQuery = baseRunQuery.Where("workspace_id = ?", *scopeID)
	case "organization":
		baseResultQuery = baseResultQuery.Joins("JOIN runs ON runs.id = run_results.run_id").
			Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Where("workspaces.organization_id = ?", *scopeID)
		baseRunQuery = baseRunQuery.Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Where("workspaces.organization_id = ?", *scopeID)
	case "global":
		// No filter
	}

	// 1. Total Runs
	baseRunQuery.Count(&stats.TotalRuns)

	// 2. Total Results
	var totalRes int64
	baseResultQuery.Session(&gorm.Session{}).Count(&totalRes)
	stats.TotalResults = totalRes

	if totalRes > 0 {
		// 3. Success Rate
		var successCount int64
		baseResultQuery.Session(&gorm.Session{}).Where("run_results.status = ?", "success").Count(&successCount)
		stats.SuccessRate = float64(successCount) / float64(totalRes)

		// 4. Avg Duration
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

	baseEvalQuery := h.db.Model(&models.Evaluation{}).
		Joins("JOIN run_results ON run_results.id = evaluations.run_result_id").
		Joins("JOIN runs ON runs.id = run_results.run_id")

	switch scope {
	case "workspace":
		baseEvalQuery = baseEvalQuery.Where("runs.workspace_id = ?", *scopeID)
	case "organization":
		baseEvalQuery = baseEvalQuery.Joins("JOIN workspaces ON workspaces.id = runs.workspace_id").
			Where("workspaces.organization_id = ?", *scopeID)
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

	// 5. Agent Performance
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

	logger.Debug("[Stats] Found %d agent rows for scope %s", len(agentRows), scope)

	for _, row := range agentRows {
		var agent models.Agent
		// Use First instead of FirstOrCreate to ensure we only get existing agents
		if err := h.db.First(&agent, "id = ?", row.AgentID).Error; err != nil {
			logger.Warn("[Stats] Agent %v not found in DB", row.AgentID)
			continue
		}

		ap := AgentPerformance{
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

		// Get owner (User of the workspace where the agent belongs)
		var workspace models.Workspace
		if h.db.Preload("User").First(&workspace, "id = ?", agent.WorkspaceID).Error == nil {
			if workspace.User.ID != uuid.Nil {
				ap.Owner = workspace.User.Name
			} else {
				ap.Owner = "System"
			}
			ap.OrgName = "" // Organizations removed
		} else {
			ap.Owner = "Unknown"
			ap.OrgName = "Unknown"
		}

		stats.Agents = append(stats.Agents, ap)
	}

	logger.Info("[Stats] Computation complete: Runs=%d, Results=%d, Agents=%d", stats.TotalRuns, stats.TotalResults, len(stats.Agents))
	return &stats, nil
}

// saveToCache stores computed stats in the database cache
func (h *StatsHandler) saveToCache(scope string, scopeID *uuid.UUID, stats *AggregatedStats, ttl time.Duration) {
	data, _ := json.Marshal(stats)
	now := time.Now().UTC()

	cache := models.StatsCache{
		ID:         uuid.New(),
		Scope:      scope,
		ScopeID:    scopeID,
		Data:       data,
		ComputedAt: now,
		ExpiresAt:  now.Add(ttl),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Upsert: delete existing and insert new
	query := h.db.Where("scope = ?", scope)
	if scopeID != nil {
		query = query.Where("scope_id = ?", *scopeID)
	} else {
		query = query.Where("scope_id IS NULL")
	}
	query.Delete(&models.StatsCache{})

	h.db.Create(&cache)
}

// agentQuery helper - kept for backwards compatibility
func (h *StatsHandler) agentQuery(where string, args []any, agentID uuid.UUID) *gorm.DB {
	q := h.db.Model(&models.RunResult{}).Where("agent_id = ?", agentID)
	if where == "workspace_id = ?" {
		q = q.Joins("JOIN runs ON runs.id = run_results.run_id").Where("runs.workspace_id = ?", args[0])
	} else if where != "1=1" {
		q = q.Where(where, args...)
	}
	return q
}
