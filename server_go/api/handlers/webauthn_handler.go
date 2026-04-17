package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

// WebAuthnLoginBegin initiates a WebAuthn login ceremony
func (h *AuthHandler) WebAuthnLoginBegin(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	options, sessionData, err := h.webauthn.BeginLogin(user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to begin login: " + err.Error()})
	}

	sessionID := uuid.New().String()
	h.sessionMu.Lock()
	h.sessions[sessionID] = sessionData
	h.sessionMu.Unlock()

	// Store sessionID in a short-lived cookie
	cookie := new(http.Cookie)
	cookie.Name = "webauthn_session"
	cookie.Value = sessionID
	cookie.Expires = time.Now().Add(5 * time.Minute)
	cookie.HttpOnly = true
	cookie.Path = "/"
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, options)
}

// WebAuthnLoginFinish completes a WebAuthn login ceremony
func (h *AuthHandler) WebAuthnLoginFinish(c echo.Context) error {
	var req struct {
		Email    string          `json:"email"`
		Response json.RawMessage `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	cookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session cookie missing"})
	}

	h.sessionMu.RLock()
	sessionData, ok := h.sessions[cookie.Value]
	h.sessionMu.RUnlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session not found or expired"})
	}

	h.sessionMu.Lock()
	delete(h.sessions, cookie.Value)
	h.sessionMu.Unlock()

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	fakeReq, err := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to build WebAuthn request"})
	}
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.webauthn.FinishLogin(user, *sessionData, fakeReq)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Failed to finish login: " + err.Error()})
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

	token, err := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),
		"", // No organization
		user.Email,
		h.jwtSecret,
		"",
	)
	if err != nil {
		logger.Error("[AUTH] Failed to generate token after passkey login for user %s: %v", user.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate authentication token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	})
}

// WebAuthnRegisterBegin initiates a WebAuthn credential registration (protected)
func (h *AuthHandler) WebAuthnRegisterBegin(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	options, sessionData, err := h.webauthn.BeginRegistration(user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to begin registration: " + err.Error()})
	}

	sessionID := uuid.New().String()
	h.sessionMu.Lock()
	h.sessions[sessionID] = sessionData
	h.sessionMu.Unlock()

	cookie := new(http.Cookie)
	cookie.Name = "webauthn_reg_session"
	cookie.Value = sessionID
	cookie.Expires = time.Now().Add(5 * time.Minute)
	cookie.HttpOnly = true
	cookie.Path = "/"
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, options)
}

// WebAuthnRegisterFinish completes a WebAuthn credential registration (protected)
func (h *AuthHandler) WebAuthnRegisterFinish(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var req struct {
		Response json.RawMessage `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	cookie, err := c.Cookie("webauthn_reg_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session cookie missing"})
	}

	h.sessionMu.RLock()
	sessionData, ok := h.sessions[cookie.Value]
	h.sessionMu.RUnlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session not found or expired"})
	}

	h.sessionMu.Lock()
	delete(h.sessions, cookie.Value)
	h.sessionMu.Unlock()

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	fakeReq, err := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to build WebAuthn request"})
	}
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.webauthn.FinishRegistration(user, *sessionData, fakeReq)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to finish registration: " + err.Error()})
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
		CreatedAt:      time.Now(),
	}

	if err := h.db.Create(&passkey).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save passkey"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Passkey registered successfully",
	})
}
