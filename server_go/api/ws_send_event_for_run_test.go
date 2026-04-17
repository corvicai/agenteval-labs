package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendEventForRun_ReachesOwnerAndCollaborator verifies that orchestrator
// events routed through SendEventForRun reach BOTH the owner's connection
// and every active collaborator's connection — regardless of each
// connection's current WorkspaceID. This is the hot path exercised by the
// engine callback in main.go after Plano 28 fixes.
func TestSendEventForRun_ReachesOwnerAndCollaborator(t *testing.T) {
	setup()

	owner, ownerWorkspace, _, qs, _ := createQuestionSetTestFixture(t, "sefr-owner")
	collab, collabWorkspace, _, _, _ := createQuestionSetTestFixture(t, "sefr-collab")

	// Simulate an accepted collaboration row. The collab's own workspace is
	// NOT the owner's — which is precisely the scenario where workspace-
	// scoped broadcasts used to fail.
	require.NoError(t, db.Create(&models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          collab.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      ptrTime(time.Now()),
	}).Error)

	// Create a run that lives in the owner's workspace but belongs to the
	// shared QS — runs always live on the owner side even when started by
	// a collaborator.
	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   ownerWorkspace.ID,
		QuestionSetID: qs.ID,
		Status:        "running",
		TotalTasks:    1,
	}
	require.NoError(t, db.Create(&run).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	ownerConn := &Connection{
		ID:              uuid.New(),
		UserID:          owner.ID,
		WorkspaceID:     ownerWorkspace.ID,
		Send:            make(chan []byte, 8),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	collabConn := &Connection{
		ID:              uuid.New(),
		UserID:          collab.ID,
		WorkspaceID:     collabWorkspace.ID, // different workspace from owner
		Send:            make(chan []byte, 8),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	hub.Register(ownerConn)
	hub.Register(collabConn)
	time.Sleep(50 * time.Millisecond)

	payload := map[string]any{
		"run_id":      run.ID.String(),
		"agent_id":    uuid.New().String(),
		"question_id": "q1",
		"success":     true,
		"answer":      "42",
	}

	require.NoError(t, hub.SendEventForRun(run.ID, EvtTaskCompleted, run.ID.String(), payload))

	assertReceives(t, "owner", ownerConn, EvtTaskCompleted, run.ID.String())
	assertReceives(t, "collaborator", collabConn, EvtTaskCompleted, run.ID.String())
}

// TestSendEventForRun_SkipsRevokedCollaborator ensures we don't leak events
// to collaborators whose access has been revoked.
func TestSendEventForRun_SkipsRevokedCollaborator(t *testing.T) {
	setup()

	owner, ownerWorkspace, _, qs, _ := createQuestionSetTestFixture(t, "sefr-owner-rev")
	revoked, revokedWorkspace, _, _, _ := createQuestionSetTestFixture(t, "sefr-revoked")

	now := time.Now()
	require.NoError(t, db.Create(&models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          revoked.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      ptrTime(now.Add(-2 * time.Hour)),
		RevokedAt:       ptrTime(now.Add(-1 * time.Hour)),
	}).Error)

	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   ownerWorkspace.ID,
		QuestionSetID: qs.ID,
		Status:        "running",
	}
	require.NoError(t, db.Create(&run).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	ownerConn := &Connection{
		ID: uuid.New(), UserID: owner.ID, WorkspaceID: ownerWorkspace.ID,
		Send: make(chan []byte, 4), Done: make(chan struct{}), IsAuthenticated: true,
	}
	revokedConn := &Connection{
		ID: uuid.New(), UserID: revoked.ID, WorkspaceID: revokedWorkspace.ID,
		Send: make(chan []byte, 4), Done: make(chan struct{}), IsAuthenticated: true,
	}
	hub.Register(ownerConn)
	hub.Register(revokedConn)
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, hub.SendEventForRun(run.ID, EvtTaskCompleted, run.ID.String(), map[string]any{"run_id": run.ID.String()}))

	// Owner must receive.
	assertReceives(t, "owner", ownerConn, EvtTaskCompleted, run.ID.String())
	// Revoked collaborator must NOT receive the task event (may still see
	// unrelated online-status envelopes emitted on registration).
	assertDoesNotReceive(t, "revoked collaborator", revokedConn, EvtTaskCompleted, 200*time.Millisecond)
}

// TestSendEventForRun_UnknownRunReturnsError ensures the fallback in the
// engine callback has a signal to switch to workspace-scoped delivery.
func TestSendEventForRun_UnknownRunReturnsError(t *testing.T) {
	setup()
	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	err := hub.SendEventForRun(uuid.New(), EvtTaskCompleted, "corr", map[string]any{})
	assert.Error(t, err)
}

func ptrTime(t time.Time) *time.Time { return &t }

// assertReceives drains unrelated envelopes (e.g. EVT_ONLINE_STATUS emitted
// on Register) until it finds one matching wantType, or times out.
func assertReceives(t *testing.T, label string, conn *Connection, wantType, wantCorr string) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case msg := <-conn.Send:
			var env models.Envelope
			require.NoError(t, json.Unmarshal(msg, &env))
			if env.Type != wantType {
				continue
			}
			assert.Equal(t, wantCorr, env.CorrelationID, "%s correlation mismatch", label)
			return
		case <-deadline:
			t.Fatalf("%s did not receive %s", label, wantType)
			return
		}
	}
}

// assertDoesNotReceive fails if conn receives an envelope of type blockType
// within the given window. Other envelope types are drained silently.
func assertDoesNotReceive(t *testing.T, label string, conn *Connection, blockType string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case msg := <-conn.Send:
			var env models.Envelope
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			if env.Type == blockType {
				t.Fatalf("%s should not have received %s", label, blockType)
				return
			}
		case <-deadline:
			return
		}
	}
}
