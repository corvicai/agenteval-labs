package api

import (
	"encoding/json"
	"os"
	"time"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (h *Hub) handleManagerGetWorkspaces(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	type WorkspaceResponse struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		UserID     uuid.UUID `json:"user_id"`
		UserName   string    `json:"user_name"`
		AgentCount int64     `json:"agent_count"`
		RunCount   int64     `json:"run_count"`
		CreatedAt  time.Time `json:"created_at"`
	}

	var workspaces []WorkspaceResponse
	h.db.Raw(`
		SELECT w.id, w.name, w.user_id, u.name as user_name, w.created_at,
		       (SELECT COUNT(*) FROM agents WHERE workspace_id = w.id) as agent_count,
		       (SELECT COUNT(*) FROM runs WHERE workspace_id = w.id) as run_count
		FROM workspaces w
		JOIN users u ON w.user_id = u.id
		WHERE w.organization_id = ?
		ORDER BY w.name
	`, orgID).Scan(&workspaces)

	c.SendResponse(DataManagerWorkspaces, env.CorrelationID, workspaces)
}

func (h *Hub) handleManagerGetAgents(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	type AgentResponse struct {
		ID            uuid.UUID `json:"id"`
		Name          string    `json:"name"`
		ProviderType  string    `json:"provider_type"`
		Enabled       bool      `json:"enabled"`
		WorkspaceID   uuid.UUID `json:"workspace_id"`
		WorkspaceName string    `json:"workspace_name"`
		UserName      string    `json:"user_name"`
		CreatedAt     time.Time `json:"created_at"`
	}

	var agents []AgentResponse
	h.db.Raw(`
		SELECT a.id, a.name, a.provider_type, a.enabled, a.workspace_id, a.created_at,
		       w.name as workspace_name, u.name as user_name
		FROM agents a
		JOIN workspaces w ON a.workspace_id = w.id
		JOIN users u ON w.user_id = u.id
		WHERE w.organization_id = ?
		ORDER BY a.name
	`, orgID).Scan(&agents)

	c.SendResponse(DataManagerAgents, env.CorrelationID, agents)
}

func (h *Hub) handleManagerGetRuns(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	type RunResponse struct {
		ID              uuid.UUID `json:"id"`
		Status          string    `json:"status"`
		WorkspaceID     uuid.UUID `json:"workspace_id"`
		WorkspaceName   string    `json:"workspace_name"`
		UserName        string    `json:"user_name"`
		QuestionSetName string    `json:"question_set_name"`
		ResultCount     int64     `json:"result_count"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var runs []RunResponse
	h.db.Raw(`
		SELECT r.id, r.status, r.workspace_id, r.created_at,
		       w.name as workspace_name, u.name as user_name,
		       qs.name as question_set_name,
		       (SELECT COUNT(*) FROM run_results WHERE run_id = r.id) as result_count
		FROM runs r
		JOIN workspaces w ON r.workspace_id = w.id
		JOIN users u ON w.user_id = u.id
		LEFT JOIN question_sets qs ON r.question_set_id = qs.id
		WHERE w.organization_id = ?
		ORDER BY r.created_at DESC
		LIMIT 100
	`, orgID).Scan(&runs)

	c.SendResponse(DataManagerRuns, env.CorrelationID, runs)
}

func (h *Hub) handleManagerGetUsers(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var users []models.UserResponse
	h.db.Raw(`
		SELECT u.id, u.name, u.email, u.is_admin, u.is_suspended, u.created_at,
		       COUNT(w.id) as workspace_count
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		WHERE uo.organization_id = ?
		GROUP BY u.id
		ORDER BY u.name
	`, orgID).Scan(&users)

	c.SendResponse(DataAdminUsers, env.CorrelationID, users)
}

func (h *Hub) handleManagerCreateUser(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var req models.ManagerCreateUserPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create user")
		return
	}

	h.db.Create(&models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: *orgID,
		Role:           "member",
		JoinedAt:       time.Now(),
	})

	h.db.Create(&models.Workspace{
		ID:             uuid.New(),
		UserID:         user.ID,
		OrganizationID: *orgID,
		Name:           "main",
	})

	c.SendResponse(DataAdminUsers, env.CorrelationID, user)
}

func (h *Hub) handleManagerUpdateUser(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var req models.ManagerUpdateUserPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user id")
		return
	}

	var user models.User
	if err := h.db.Raw(`
		SELECT u.* FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		WHERE u.id = ? AND uo.organization_id = ?
	`, targetUID, orgID).Scan(&user).Error; err != nil || user.ID == uuid.Nil {
		c.SendError(env.CorrelationID, "user not found in your organization")
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		user.Email = req.Email
	}

	if err := h.db.Save(&user).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to update user")
		return
	}

	c.SendResponse(DataAdminUsers, env.CorrelationID, user)
}

func (h *Hub) handleManagerToggleUserSuspension(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	if targetUID == c.UserID {
		c.SendError(env.CorrelationID, "cannot suspend yourself")
		return
	}

	var user models.User
	if err := h.db.Raw(`
		SELECT u.* FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		WHERE u.id = ? AND uo.organization_id = ?
	`, targetUID, orgID).Scan(&user).Error; err != nil || user.ID == uuid.Nil {
		c.SendError(env.CorrelationID, "user not found in your organization")
		return
	}

	user.IsSuspended = !user.IsSuspended
	h.db.Save(&user)

	if user.IsSuspended {
		evtPayload := map[string]string{"reason": "suspended_by_manager"}
		h.BroadcastToUser(user.ID, func() []byte {
			b, _ := json.Marshal(models.Envelope{
				Type:    EvtForceLogout,
				Payload: createJSONPayload(evtPayload),
			})
			return b
		}())
	}

	c.SendResponse(DataAdminUsers, env.CorrelationID, user)
}

func (h *Hub) handleManagerImpersonateUser(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var req models.ManagerImpersonatePayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid target id")
		return
	}

	if targetUID == c.UserID {
		c.SendError(env.CorrelationID, "cannot impersonate yourself")
		return
	}

	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", targetUID).Error; err != nil {
		c.SendError(env.CorrelationID, "target user not found")
		return
	}

	var userOrg models.UserOrganization
	if err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", targetUID, orgID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not in your organization")
		return
	}

	var workspace models.Workspace
	h.db.Where("user_id = ? AND organization_id = ?", targetUID, orgID).First(&workspace)

	workspaceID := ""
	if workspace.ID != uuid.Nil {
		workspaceID = workspace.ID.String()
	}

	token, err := middleware.GenerateToken(
		targetUID.String(),
		workspaceID,
		orgID.String(),
		targetUser.Email,
		os.Getenv("JWT_SECRET"), // Correctly get from env
		c.UserID.String(),
	)

	if err != nil {
		c.SendError(env.CorrelationID, "failed to generate token")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"token":     token,
		"user":      targetUser,
		"workspace": workspace,
	})
}

func (h *Hub) handleManagerGetStats(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var stats struct {
		UserCount      int64 `json:"user_count"`
		WorkspaceCount int64 `json:"workspace_count"`
		AgentCount     int64 `json:"agent_count"`
		RunCount       int64 `json:"run_count"`
	}

	h.db.Raw(`SELECT COUNT(*) FROM user_organizations WHERE organization_id = ?`, orgID).Scan(&stats.UserCount)
	h.db.Raw(`SELECT COUNT(*) FROM workspaces WHERE organization_id = ?`, orgID).Scan(&stats.WorkspaceCount)
	h.db.Raw(`SELECT COUNT(*) FROM agents a JOIN workspaces w ON a.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.AgentCount)
	h.db.Raw(`SELECT COUNT(*) FROM runs r JOIN workspaces w ON r.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.RunCount)

	c.SendResponse(DataManagerStats, env.CorrelationID, stats)
}

// handleManagerGenerateInvite creates an invite code for the manager's organization
func (h *Hub) handleManagerGenerateInvite(c *Connection, env models.Envelope) {
	orgID, err := h.verifyManager(c)
	if err != nil {
		c.SendError(env.CorrelationID, err.Error())
		return
	}

	var payload struct {
		MaxUses int `json:"max_uses"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if payload.MaxUses <= 0 {
		payload.MaxUses = 1
	}

	code := generateRandomCode(8)
	invite := models.InviteCode{
		Code:           code,
		CreatedBy:      c.UserID,
		OrganizationID: orgID,
		Role:           "member", // Managers invite members
		IsNewOrg:       false,
		ExpiresAt:      time.Now().Add(24 * time.Hour * 7),
		MaxUses:        payload.MaxUses,
		UseCount:       0,
		CreatedAt:      time.Now(),
	}

	if err := h.db.Create(&invite).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create invite: "+err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"code":       invite.Code,
		"expires_at": invite.ExpiresAt,
	})
}
