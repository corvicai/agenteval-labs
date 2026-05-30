package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// Workspace stats must only be readable by the owner of that workspace (or an
// admin), not by any authenticated user who supplies an arbitrary workspace_id.
func TestGetWorkspaceStatsAccessControl(t *testing.T) {
	setup()
	owner, ownerToken := createTestUser(t, false)
	_, attackerToken := createTestUser(t, false)

	ws := models.Workspace{ID: uuid.New(), UserID: owner.ID, Name: "WS"}
	if err := db.Create(&ws).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	payload := map[string]any{"workspace_id": ws.ID.String(), "force": true}

	t.Run("OwnerAllowed", func(t *testing.T) {
		resp := sendWSRequest(t, ownerToken, ReqGetWorkspaceStats, payload)
		assert.Equal(t, DataWorkspaceStats, resp.Type)
	})

	t.Run("OtherUserDenied", func(t *testing.T) {
		resp := sendWSRequest(t, attackerToken, ReqGetWorkspaceStats, payload)
		assert.Equal(t, EvtError, resp.Type)
		var p map[string]string
		json.Unmarshal(resp.Payload, &p)
		assert.Contains(t, p["error"], "access denied")
	})
}
