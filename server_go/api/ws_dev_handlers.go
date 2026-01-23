package api

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

// handleDevGetManagers returns all managers and admins for the quick login panel.
// Dev-only.
func (h *Hub) handleDevGetManagers(c *Connection, env models.Envelope) {
	if os.Getenv("APP_ENV") == "production" {
		c.SendError(env.CorrelationID, "not available in production")
		return
	}

	var managers []models.ManagerInfo
	h.db.Raw(`
		SELECT u.id, u.name, u.email, o.name as org_name
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		JOIN organizations o ON uo.organization_id = o.id
		WHERE uo.role IN ('admin', 'manager')
		ORDER BY o.name, u.name
	`).Scan(&managers)

	c.SendResponse(DataDevManagers, env.CorrelationID, managers)
}

// handleDevLogin performs a passwordless login for development.
// Dev-only.
func (h *Hub) handleDevLogin(c *Connection, env models.Envelope) {
	if os.Getenv("APP_ENV") == "production" {
		c.SendError(env.CorrelationID, "not available in production")
		return
	}

	var req models.DevLoginPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user_id")
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	var userOrg models.UserOrganization
	if err := h.db.Preload("Organization").First(&userOrg, "user_id = ?", userID).Error; err != nil {
		c.SendError(env.CorrelationID, "user has no organization")
		return
	}

	var workspace models.Workspace
	h.db.Preload("User").Preload("Organization").Where("user_id = ? AND organization_id = ?", userID, userOrg.OrganizationID).First(&workspace)

	// Update connection
	c.UserID = user.ID
	c.OrgID = userOrg.OrganizationID
	c.WorkspaceID = workspace.ID
	c.IsAuthenticated = true

	// Generate token
	token, _ := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),
		userOrg.OrganizationID.String(),
		user.Email,
		os.Getenv("JWT_SECRET"),
		"",
	)

	result := map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":       user.ID.String(),
			"name":     user.Name,
			"email":    user.Email,
			"is_admin": user.IsAdmin,
		},
		"organization": map[string]any{
			"id":   userOrg.Organization.ID.String(),
			"name": userOrg.Organization.Name,
		},
		"workspace": workspace,
	}

	c.SendResponse(DataDevLoginResult, env.CorrelationID, result)
}

// handleSeedHistoricalRun backfills a run with results and evaluations.
// Dev/Maintenance only.
func (h *Hub) handleSeedHistoricalRun(c *Connection, env models.Envelope) {
	if os.Getenv("APP_ENV") == "production" {
		log.Printf("[WS][SECURITY] Attempted SeedHistoricalRun in production by user %s", c.UserID)
		c.SendError(env.CorrelationID, "CMD_SEED_HISTORICAL_RUN is disabled in production")
		return
	}

	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload models.SeedHistoricalRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	qsID, _ := uuid.Parse(payload.QuestionSetID)
	targetWSID := c.WorkspaceID
	if payload.WorkspaceID != "" {
		if id, err := uuid.Parse(payload.WorkspaceID); err == nil {
			targetWSID = id
		}
	}

	// Validate question set belongs to target workspace (via client)
	var qs models.QuestionSet
	if err := h.db.Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("question_sets.id = ? AND clients.workspace_id = ?", qsID, targetWSID).
		First(&qs).Error; err != nil {
		c.SendError(env.CorrelationID, "question set not found in workspace")
		return
	}

	// Validate all agents belong to target workspace
	agentIDs := make(map[uuid.UUID]bool)
	for _, res := range payload.Results {
		if id, err := uuid.Parse(res.AgentID); err == nil {
			agentIDs[id] = true
		}
	}
	if len(agentIDs) > 0 {
		var count int64
		var ids []uuid.UUID
		for id := range agentIDs {
			ids = append(ids, id)
		}
		if err := h.db.Model(&models.Agent{}).
			Where("workspace_id = ? AND id IN ?", targetWSID, ids).
			Count(&count).Error; err != nil || count != int64(len(agentIDs)) {
			c.SendError(env.CorrelationID, "one or more agents not found in workspace")
			return
		}
	}

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   targetWSID,
		QuestionSetID: qsID,
		Status:        "completed",
		TotalTasks:    len(payload.Results),
		CreatedAt:     payload.CreatedAt,
	}

	if err := h.db.Create(&run).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create run: "+err.Error())
		return
	}

	var results []models.RunResult
	var evaluations []models.Evaluation

	for _, resP := range payload.Results {
		aID, _ := uuid.Parse(resP.AgentID)
		resultID := uuid.New()

		result := models.RunResult{
			ID:         resultID,
			RunID:      run.ID,
			AgentID:    aID,
			QuestionID: resP.QuestionID,
			Status:     resP.Status,
			Answer:     resP.Answer,
			DurationMs: resP.DurationMs,
			CreatedAt:  payload.CreatedAt,
		}
		results = append(results, result)

		for _, evP := range resP.Evaluations {
			ratingCode := evP.RatingCode
			// Backfill ratingCode if only Rating (string) is provided
			if ratingCode == nil {
				var code int
				switch evP.Rating {
				case "like":
					code = 1
				case "valid":
					code = 2
				case "dislike":
					code = 3
				case "wrong":
					code = 4
				}
				if code > 0 {
					ratingCode = &code
				}
			}

			score := evP.Score
			if score == nil && ratingCode != nil {
				defaultScore := 0
				switch *ratingCode {
				case 1:
					defaultScore = 100
				case 2:
					defaultScore = 75
				case 3:
					defaultScore = 25
				case 4:
					defaultScore = 0
				}
				score = &defaultScore
			}

			eval := models.Evaluation{
				ID:          uuid.New(),
				RunResultID: resultID,
				RaterType:   evP.RaterType,
				Rating:      evP.Rating,
				RatingCode:  ratingCode,
				Score:       score,
				Comments:    evP.Comments,
				CreatedAt:   payload.CreatedAt,
			}
			evaluations = append(evaluations, eval)
		}
	}

	if len(results) > 0 {
		if err := h.db.CreateInBatches(results, 100).Error; err != nil {
			log.Printf("[WS] Seed failed to batch insert results: %v", err)
			c.SendError(env.CorrelationID, "failed to insert results")
			return
		}
	}

	if len(evaluations) > 0 {
		if err := h.db.CreateInBatches(evaluations, 100).Error; err != nil {
			log.Printf("[WS] Seed failed to batch insert evaluations: %v", err)
		}
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "seeded", "run_id": run.ID.String()})
}

// handleAdminRecalculateStats clears the stats cache and triggers a re-aggregation.
// Admin/Maintenance only.
func (h *Hub) handleAdminRecalculateStats(c *Connection, env models.Envelope) {
	// 1. STRICT Safeguard: Production check
	if os.Getenv("APP_ENV") == "production" {
		log.Printf("[WS][SECURITY] Attempted AdminRecalculateStats in production by user %s", c.UserID)
		c.SendError(env.CorrelationID, "CMD_ADMIN_RECALCULATE_STATS is disabled in production")
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil || !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	log.Printf("[WS] Triggering global stats recalculation (Admin: %s)", c.UserID)

	if err := h.db.Exec("TRUNCATE TABLE stats_cache").Error; err != nil {
		c.SendError(env.CorrelationID, "failed to clear cache: "+err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "stats cache cleared"})
}

func (h *Hub) handleCheckDBPerf(c *Connection, env models.Envelope) {
	if os.Getenv("APP_ENV") == "production" {
		c.SendError(env.CorrelationID, "not available in production")
		return
	}

	start := time.Now()
	var result int
	if err := h.db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		c.SendError(env.CorrelationID, "DB check failed: "+err.Error())
		return
	}
	duration := time.Since(start).Milliseconds()
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":      "ok",
		"duration_ms": duration,
	})
}
