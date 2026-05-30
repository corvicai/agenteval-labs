package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

// seedRunOwnedBy creates workspace → client → question set → run → result owned
// by ownerID, and returns the run id and the (secret) answer stored on the result.
func seedRunOwnedBy(t *testing.T, ownerID uuid.UUID) (runID uuid.UUID, secretAnswer string) {
	t.Helper()
	ws := models.Workspace{ID: uuid.New(), UserID: ownerID, Name: "WS-" + ownerID.String()}
	client := models.Client{ID: uuid.New(), WorkspaceID: ws.ID, Name: "Client"}
	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: datatypes.JSON([]byte("[]"))}
	run := models.Run{ID: uuid.New(), WorkspaceID: ws.ID, QuestionSetID: qs.ID, Status: "completed"}
	secretAnswer = "secret-answer-" + uuid.New().String()
	result := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    uuid.New(),
		QuestionID: "q1",
		Status:     "success",
		Answer:     secretAnswer,
	}
	for _, m := range []any{&ws, &client, &qs, &run, &result} {
		if err := db.Create(m).Error; err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}
	return run.ID, secretAnswer
}

// A run (and its results) must only be readable by the owner of its question
// set's workspace or an accepted collaborator — never by an arbitrary
// authenticated user who knows/guesses the run ID (cross-tenant IDOR).
func TestRunReadAccessControl(t *testing.T) {
	setup()
	owner, ownerToken := createTestUser(t, false)
	_, attackerToken := createTestUser(t, false)
	runID, secret := seedRunOwnedBy(t, owner.ID)

	idPayload := map[string]string{"run_id": runID.String()}

	t.Run("GetRunDetails_OwnerAllowed", func(t *testing.T) {
		resp := sendWSRequest(t, ownerToken, ReqGetRunDetails, idPayload)
		assert.Equal(t, DataRunDetails, resp.Type)
	})
	t.Run("GetRunDetails_OtherDenied", func(t *testing.T) {
		resp := sendWSRequest(t, attackerToken, ReqGetRunDetails, idPayload)
		assert.Equal(t, EvtError, resp.Type)
		var p map[string]string
		json.Unmarshal(resp.Payload, &p)
		assert.Contains(t, p["error"], "access denied")
	})

	t.Run("GetRunLite_OtherDenied", func(t *testing.T) {
		resp := sendWSRequest(t, attackerToken, ReqGetRunLite, idPayload)
		assert.Equal(t, EvtError, resp.Type)
	})

	t.Run("GetResultDetails_DoesNotLeakToOther", func(t *testing.T) {
		// Owner sees the answer; the attacker must not (filtered out).
		ownerResp := sendWSRequest(t, ownerToken, ReqGetResultDetails,
			map[string][]string{"result_ids": {seedResultID(t, runID)}})
		assert.Contains(t, string(ownerResp.Payload), secret)

		attackerResp := sendWSRequest(t, attackerToken, ReqGetResultDetails,
			map[string][]string{"result_ids": {seedResultID(t, runID)}})
		assert.NotContains(t, string(attackerResp.Payload), secret)
	})
}

// seedResultID returns the id of a result belonging to runID.
func seedResultID(t *testing.T, runID uuid.UUID) string {
	t.Helper()
	var r models.RunResult
	if err := db.First(&r, "run_id = ?", runID).Error; err != nil {
		t.Fatalf("no result for run: %v", err)
	}
	return r.ID.String()
}
