package api

import (
	"encoding/json"
	"fmt"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
)

func (h *Hub) getManagerOrgID(userID uuid.UUID, orgID uuid.UUID) (*uuid.UUID, error) {
	if userID == uuid.Nil || orgID == uuid.Nil {
		return nil, fmt.Errorf("missing user or organization context")
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if user.IsAdmin {
		return &orgID, nil
	}

	var userOrg models.UserOrganization
	if err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", userID, orgID).Error; err != nil {
		return nil, fmt.Errorf("user is not a member of this organization")
	}

	if userOrg.Role != "manager" {
		return nil, fmt.Errorf("not a manager of this organization")
	}

	return &orgID, nil
}

func (h *Hub) handleGetManagerStats(c *Connection, env models.Envelope) {
	orgID, err := h.getManagerOrgID(c.UserID, c.OrgID)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var stats models.ManagerStatsPayload
	h.db.Raw(`SELECT COUNT(*) FROM user_organizations WHERE organization_id = ?`, orgID).Scan(&stats.UserCount)
	h.db.Raw(`SELECT COUNT(*) FROM workspaces WHERE organization_id = ?`, orgID).Scan(&stats.WorkspaceCount)
	h.db.Raw(`SELECT COUNT(*) FROM agents a JOIN workspaces w ON a.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.AgentCount)
	h.db.Raw(`SELECT COUNT(*) FROM runs r JOIN workspaces w ON r.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.RunCount)

	c.SendResponse(DataManagerStats, env.CorrelationID, stats)
}

func (h *Hub) handleGetManagerUsers(c *Connection, env models.Envelope) {
	orgID, err := h.getManagerOrgID(c.UserID, c.OrgID)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var users []models.UserResponse
	h.db.Raw(`
		SELECT u.id, u.name, u.email,
		       CASE WHEN u.is_admin = true OR LOWER(u.email) = ? THEN true ELSE false END as is_admin,
		       u.is_suspended,
		       COUNT(w.id) as workspace_count
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		WHERE uo.organization_id = ?
		GROUP BY u.id
		ORDER BY u.name
	`, models.HardcodedAdminEmail(), orgID).Scan(&users)

	c.SendResponse(DataManagerUsers, env.CorrelationID, users)
}

func (h *Hub) handleGetWorkspaceStats(c *Connection, env models.Envelope) {
	var req models.GetWorkspaceStatsPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	wsID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid workspace_id")
		return
	}

	// Authorize: only the workspace owner (or an admin) may read its stats.
	if _, err := h.loadOwnedWorkspace(h.db, c.UserID, wsID); err != nil {
		var user models.User
		if dbErr := h.db.First(&user, "id = ?", c.UserID).Error; dbErr != nil || !user.IsAdmin {
			c.SendError(env.CorrelationID, "access denied")
			return
		}
	}

	const WorkspaceCacheTTL = 5 * time.Minute

	if !req.Force {
		var cache models.StatsCache
		if err := h.db.Where("scope = ? AND scope_id = ?", "workspace", wsID).First(&cache).Error; err == nil {
			if time.Now().UTC().Before(cache.ExpiresAt) {
				var stats models.AggregatedStats
				if err := json.Unmarshal(cache.Data, &stats); err == nil {
					stats.CacheHit = true
					stats.ComputedAt = cache.ComputedAt
					c.SendResponse(DataWorkspaceStats, env.CorrelationID, stats)
					return
				}
			}
		}
	}

	stats, err := h.statsService.ComputeStats("workspace", &wsID)
	if err != nil {
		c.SendError(env.CorrelationID, "failed to compute stats: "+err.Error())
		return
	}

	// Save to Cache
	data, _ := json.Marshal(stats)
	now := time.Now().UTC()
	cache := models.StatsCache{
		ID:         uuid.New(),
		Scope:      "workspace",
		ScopeID:    &wsID,
		Data:       data,
		ComputedAt: now,
		ExpiresAt:  now.Add(WorkspaceCacheTTL),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	h.db.Where("scope = ? AND scope_id = ?", "workspace", wsID).Delete(&models.StatsCache{})
	h.db.Create(&cache)

	c.SendResponse(DataWorkspaceStats, env.CorrelationID, stats)
}

func (h *Hub) handleGetOrgStats(c *Connection, env models.Envelope) {
	var req models.GetOrgStatsPayload
	// Optional payload: missing/invalid yields zero-value with force=false.
	_ = json.Unmarshal([]byte(env.Payload), &req) //nolint:errcheck // payload is optional

	// Need orgID from connection
	orgID := c.OrgID
	if orgID == uuid.Nil {
		c.SendError(env.CorrelationID, "organization context missing")
		return
	}

	const OrgCacheTTL = 15 * time.Minute

	if !req.Force {
		var cache models.StatsCache
		if err := h.db.Where("scope = ? AND scope_id = ?", "organization", orgID).First(&cache).Error; err == nil {
			if time.Now().UTC().Before(cache.ExpiresAt) {
				var stats models.AggregatedStats
				if err := json.Unmarshal(cache.Data, &stats); err == nil {
					stats.CacheHit = true
					stats.ComputedAt = cache.ComputedAt
					c.SendResponse(DataOrgStats, env.CorrelationID, stats)
					return
				}
			}
		}
	}

	stats, err := h.statsService.ComputeStats("organization", &orgID)
	if err != nil {
		c.SendError(env.CorrelationID, "failed to compute stats: "+err.Error())
		return
	}

	// Save to Cache
	data, _ := json.Marshal(stats)
	now := time.Now().UTC()
	cache := models.StatsCache{
		ID:         uuid.New(),
		Scope:      "organization",
		ScopeID:    &orgID,
		Data:       data,
		ComputedAt: now,
		ExpiresAt:  now.Add(OrgCacheTTL),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	h.db.Where("scope = ? AND scope_id = ?", "organization", orgID).Delete(&models.StatsCache{})
	h.db.Create(&cache)

	c.SendResponse(DataOrgStats, env.CorrelationID, stats)
}

func (h *Hub) handleGetGlobalStats(c *Connection, env models.Envelope) {
	var req models.GetGlobalStatsPayload
	// Optional payload: missing/invalid yields zero-value with force=false.
	_ = json.Unmarshal([]byte(env.Payload), &req) //nolint:errcheck // payload is optional

	// Admin check
	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil || !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	const GlobalCacheTTL = 30 * time.Minute

	if !req.Force {
		var cache models.StatsCache
		if err := h.db.Where("scope = ? AND scope_id IS NULL", "global").First(&cache).Error; err == nil {
			if time.Now().UTC().Before(cache.ExpiresAt) {
				var stats models.AggregatedStats
				if err := json.Unmarshal(cache.Data, &stats); err == nil {
					stats.CacheHit = true
					stats.ComputedAt = cache.ComputedAt
					c.SendResponse(DataGlobalStats, env.CorrelationID, stats)
					return
				}
			}
		}
	}

	stats, err := h.statsService.ComputeStats("global", nil)
	if err != nil {
		c.SendError(env.CorrelationID, "failed to compute stats: "+err.Error())
		return
	}

	// Save to Cache
	data, _ := json.Marshal(stats)
	now := time.Now().UTC()
	cache := models.StatsCache{
		ID:         uuid.New(),
		Scope:      "global",
		ScopeID:    nil,
		Data:       data,
		ComputedAt: now,
		ExpiresAt:  now.Add(GlobalCacheTTL),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	h.db.Where("scope = ? AND scope_id IS NULL", "global").Delete(&models.StatsCache{})
	h.db.Create(&cache)

	c.SendResponse(DataGlobalStats, env.CorrelationID, stats)
}
