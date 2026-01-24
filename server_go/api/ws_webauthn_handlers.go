package api

import (
	"benchmarking-platform/models"
	"encoding/json"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

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
