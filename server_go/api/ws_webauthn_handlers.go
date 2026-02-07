package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

func (h *Hub) handleWebAuthnRegisterBegin(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "Authentication required")
		return
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "User not found")
		return
	}

	options, sessionData, err := h.WebAuthn.BeginRegistration(user)
	if err != nil {
		c.SendError(env.CorrelationID, "Failed to begin registration: "+err.Error())
		return
	}

	sessionID := uuid.New().String()
	h.mu.Lock()
	h.webauthnSessions[sessionID] = sessionData
	h.mu.Unlock()

	c.SendResponse(DataWebAuthnRegisterOptions, env.CorrelationID, map[string]any{
		"options":    options,
		"session_id": sessionID,
	})
}

func (h *Hub) handleWebAuthnRegisterFinish(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "Authentication required")
		return
	}

	var req struct {
		SessionID string          `json:"session_id"`
		Response  json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		c.SendError(env.CorrelationID, "Invalid payload")
		return
	}

	h.mu.Lock()
	sessionData, ok := h.webauthnSessions[req.SessionID]
	if ok {
		delete(h.webauthnSessions, req.SessionID)
	}
	h.mu.Unlock()

	if !ok {
		c.SendError(env.CorrelationID, "Session not found or expired")
		return
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "User not found")
		return
	}

	fakeReq, _ := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.WebAuthn.FinishRegistration(user, *sessionData, fakeReq)
	if err != nil {
		c.SendError(env.CorrelationID, "Failed to finish registration: "+err.Error())
		return
	}

	passkey := models.Passkey{
		ID:             uuid.New(),
		UserID:         user.ID,
		CredentialID:   credential.ID,
		PublicKey:      credential.PublicKey,
		Attestation:    credential.AttestationType,
		SignCount:      credential.Authenticator.SignCount,
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.db.Create(&passkey).Error; err != nil {
		c.SendError(env.CorrelationID, "Failed to save passkey")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"success": true,
		"message": "Passkey registered successfully",
	})
}

func (h *Hub) handleWebAuthnLoginBegin(c *Connection, env models.Envelope) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		c.SendError(env.CorrelationID, "Invalid payload")
		return
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		c.SendError(env.CorrelationID, "User not found")
		return
	}

	options, sessionData, err := h.WebAuthn.BeginLogin(user)
	if err != nil {
		c.SendError(env.CorrelationID, "Failed to begin login: "+err.Error())
		return
	}

	sessionID := uuid.New().String()
	h.mu.Lock()
	h.webauthnSessions[sessionID] = sessionData
	h.mu.Unlock()

	c.SendResponse(DataWebAuthnLoginOptions, env.CorrelationID, map[string]any{
		"options":    options,
		"session_id": sessionID,
	})
}

func (h *Hub) handleWebAuthnLoginFinish(c *Connection, env models.Envelope) {
	var req struct {
		Email     string          `json:"email"`
		SessionID string          `json:"session_id"`
		Response  json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		c.SendError(env.CorrelationID, "Invalid payload")
		return
	}

	h.mu.Lock()
	sessionData, ok := h.webauthnSessions[req.SessionID]
	if ok {
		delete(h.webauthnSessions, req.SessionID)
	}
	h.mu.Unlock()

	if !ok {
		c.SendError(env.CorrelationID, "Session not found or expired")
		return
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		c.SendError(env.CorrelationID, "User not found")
		return
	}

	fakeReq, _ := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.WebAuthn.FinishLogin(user, *sessionData, fakeReq)
	if err != nil {
		c.SendError(env.CorrelationID, "Failed to finish login: "+err.Error())
		return
	}

	// Update sign count
	for i, pk := range user.Passkeys {
		if bytes.Equal(pk.CredentialID, credential.ID) {
			h.db.Model(&user.Passkeys[i]).Updates(map[string]any{
				"sign_count":      credential.Authenticator.SignCount,
				"backup_eligible": credential.Flags.BackupEligible,
				"backup_state":    credential.Flags.BackupState,
			})
			break
		}
	}

	now := time.Now().UTC()
	h.db.Model(&user).Update("last_login_at", &now)

	var workspace models.Workspace
	h.db.Preload("User").Where("user_id = ?", user.ID).First(&workspace)

	token, _ := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),
		"", // No organization
		user.Email,
		h.jwtSecret,
		"",
	)

	// Since we are in WebSocket, we can't easily set a cookie that the browser will use for REST calls automatically
	// BUT the client can store it in localStorage if they choice.
	// For consistency with REST login, we return it.

	c.SendResponse(DataWsLoginResult, env.CorrelationID, map[string]any{
		"token":     token,
		"user":      user,
		"workspace": workspace,
	})
}

func (h *Hub) handleWebAuthnDeleteKey(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "Authentication required")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		c.SendError(env.CorrelationID, "Invalid payload")
		return
	}

	keyID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "Invalid key ID")
		return
	}

	// Fetch user to check admin status
	var user models.User
	h.db.First(&user, "id = ?", c.UserID)

	// Ensure the key belongs to the user, OR the user is an admin
	var passkey models.Passkey
	if err := h.db.First(&passkey, "id = ?", keyID).Error; err != nil {
		c.SendError(env.CorrelationID, "Passkey not found")
		return
	}

	if passkey.UserID != c.UserID && !user.IsAdmin {
		c.SendError(env.CorrelationID, "Access denied")
		return
	}

	if err := h.db.Delete(&passkey).Error; err != nil {
		c.SendError(env.CorrelationID, "Failed to delete passkey")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"success": true,
		"message": "Passkey deleted successfully",
	})
}

// Ensure interface compliance
var _ webauthn.User = (*models.User)(nil)
