package api

import (
	"encoding/json"
	"testing"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestCreateCollabInvite_OwnerCanCreate
// ---------------------------------------------------------------------------

func TestCreateCollabInvite_OwnerCanCreate(t *testing.T) {
	setup()

	owner, _, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-owner-create")

	resp := sendWSRequest(t, ownerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
		"invited_email":   "collaborator@example.com",
		"role":            "editor",
	})
	assert.Equal(t, DataResponse, resp.Type)

	var payload struct {
		Token         string    `json:"token"`
		ExpiresAt     time.Time `json:"expires_at"`
		InvitedEmail  string    `json:"invited_email"`
		Role          string    `json:"role"`
		QuestionSetID string    `json:"question_set_id"`
	}
	decodeWSResponsePayload(t, resp, &payload)
	assert.NotEmpty(t, payload.Token)
	assert.Equal(t, "collaborator@example.com", payload.InvitedEmail)
	assert.Equal(t, "editor", payload.Role)
	assert.Equal(t, qs.ID.String(), payload.QuestionSetID)
	assert.True(t, payload.ExpiresAt.After(time.Now()))

	// Verify stored in DB
	var invite models.QuestionSetCollabInvite
	require.NoError(t, db.Where("token = ?", payload.Token).First(&invite).Error)
	assert.Equal(t, qs.ID, invite.QuestionSetID)
	assert.Equal(t, owner.ID, invite.CreatedByUserID)
	_ = owner
}

// ---------------------------------------------------------------------------
// TestCreateCollabInvite_NonOwnerRejected
// ---------------------------------------------------------------------------

func TestCreateCollabInvite_NonOwnerRejected(t *testing.T) {
	setup()

	_, _, _, qs, _ := createQuestionSetTestFixture(t, "collab-nonowner-qs")
	_, _, _, _, nonOwnerToken := createQuestionSetTestFixture(t, "collab-nonowner-actor")

	resp := sendWSRequest(t, nonOwnerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
	})
	assert.Equal(t, EvtError, resp.Type)

	var errPayload map[string]any
	decodeWSResponsePayload(t, resp, &errPayload)
	assert.Contains(t, errPayload["error"], "owner")
}

// ---------------------------------------------------------------------------
// TestAcceptCollabInvite_AddsCollaborator_NoClone
// ---------------------------------------------------------------------------

func TestAcceptCollabInvite_AddsCollaborator_NoClone(t *testing.T) {
	setup()

	owner, ownerWorkspace, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-accept-owner")
	collab, _, _, _, collabToken := createQuestionSetTestFixture(t, "collab-accept-collab")

	// Create invite
	createResp := sendWSRequest(t, ownerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
	})
	require.Equal(t, DataResponse, createResp.Type)

	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)
	require.NotEmpty(t, created.Token)

	// Accept invite
	acceptResp := sendWSRequest(t, collabToken, ReqAcceptQuestionSetCollabInvite, map[string]any{
		"token": created.Token,
	})
	require.Equal(t, DataResponse, acceptResp.Type)

	var accepted struct {
		QuestionSetID   string `json:"question_set_id"`
		Role            string `json:"role"`
		OwnerWorkspaceID string `json:"owner_workspace_id"`
	}
	decodeWSResponsePayload(t, acceptResp, &accepted)
	assert.Equal(t, qs.ID.String(), accepted.QuestionSetID)
	assert.Equal(t, "editor", accepted.Role)
	assert.Equal(t, ownerWorkspace.ID.String(), accepted.OwnerWorkspaceID)

	// The original QS must not have been cloned — only one QS should exist
	// belonging to the owner workspace.
	var qsCount int64
	db.Model(&models.QuestionSet{}).
		Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("clients.workspace_id = ?", ownerWorkspace.ID).
		Count(&qsCount)
	assert.Equal(t, int64(1), qsCount, "no clones should be created")

	// Collaborator row must exist
	var collaborator models.QuestionSetCollaborator
	require.NoError(t, db.Where("question_set_id = ? AND user_id = ?", qs.ID, collab.ID).First(&collaborator).Error)
	assert.NotNil(t, collaborator.AcceptedAt)
	assert.Nil(t, collaborator.RevokedAt)
	assert.Equal(t, "editor", collaborator.Role)

	// Invite must be marked accepted
	var invite models.QuestionSetCollabInvite
	require.NoError(t, db.Where("token = ?", created.Token).First(&invite).Error)
	assert.NotNil(t, invite.AcceptedAt)

	_ = owner
}

// ---------------------------------------------------------------------------
// TestAcceptCollabInvite_ExpiredRejected
// ---------------------------------------------------------------------------

func TestAcceptCollabInvite_ExpiredRejected(t *testing.T) {
	setup()

	owner, _, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-expired-owner")
	_, _, _, _, collabToken := createQuestionSetTestFixture(t, "collab-expired-collab")

	// Manually create an expired invite
	token, err := models.GenerateQuestionSetShareToken()
	require.NoError(t, err)
	invite := models.QuestionSetCollabInvite{
		ID:              uuid.New(),
		Token:           token,
		QuestionSetID:   qs.ID,
		CreatedByUserID: owner.ID,
		Role:            "editor",
		ExpiresAt:       time.Now().Add(-1 * time.Hour), // Already expired
	}
	require.NoError(t, db.Create(&invite).Error)
	_ = ownerToken

	resp := sendWSRequest(t, collabToken, ReqAcceptQuestionSetCollabInvite, map[string]any{
		"token": token,
	})
	assert.Equal(t, EvtError, resp.Type)

	var errPayload map[string]any
	decodeWSResponsePayload(t, resp, &errPayload)
	assert.Contains(t, errPayload["error"], "expired")
}

// ---------------------------------------------------------------------------
// TestAcceptCollabInvite_Idempotent
// ---------------------------------------------------------------------------

func TestAcceptCollabInvite_Idempotent(t *testing.T) {
	setup()

	owner, _, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-idempotent-owner")
	collab, _, _, _, collabToken := createQuestionSetTestFixture(t, "collab-idempotent-collab")

	// Create invite
	createResp := sendWSRequest(t, ownerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
	})
	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)

	// Accept first time
	resp1 := sendWSRequest(t, collabToken, ReqAcceptQuestionSetCollabInvite, map[string]any{
		"token": created.Token,
	})
	assert.Equal(t, DataResponse, resp1.Type)

	// Create a second invite (same QS, different token) and accept again.
	// The collaborator row should be upserted, not duplicated.
	createResp2 := sendWSRequest(t, ownerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
	})
	var created2 struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp2, &created2)

	resp2 := sendWSRequest(t, collabToken, ReqAcceptQuestionSetCollabInvite, map[string]any{
		"token": created2.Token,
	})
	assert.Equal(t, DataResponse, resp2.Type)

	// Only one collaborator row must exist
	var count int64
	db.Model(&models.QuestionSetCollaborator{}).
		Where("question_set_id = ? AND user_id = ?", qs.ID, collab.ID).
		Count(&count)
	assert.Equal(t, int64(1), count, "upsert must not create duplicates")

	_ = owner
}

// ---------------------------------------------------------------------------
// TestRevokeCollaborator_RemovesAccess
// ---------------------------------------------------------------------------

func TestRevokeCollaborator_RemovesAccess(t *testing.T) {
	setup()

	owner, _, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-revoke-owner")
	collab, _, _, _, collabToken := createQuestionSetTestFixture(t, "collab-revoke-collab")

	// Add collaborator directly in DB
	now := time.Now()
	collaborator := models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          collab.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}
	require.NoError(t, db.Create(&collaborator).Error)

	// Revoke
	resp := sendWSRequest(t, ownerToken, ReqRevokeQuestionSetCollaborator, map[string]any{
		"question_set_id": qs.ID.String(),
		"user_id":         collab.ID.String(),
	})
	assert.Equal(t, DataResponse, resp.Type)

	var revokeResult struct {
		Status string `json:"status"`
	}
	decodeWSResponsePayload(t, resp, &revokeResult)
	assert.Equal(t, "revoked", revokeResult.Status)

	// Row must have revoked_at set
	var stored models.QuestionSetCollaborator
	require.NoError(t, db.First(&stored, collaborator.ID).Error)
	assert.NotNil(t, stored.RevokedAt)

	// Non-owner trying to revoke must fail
	nonOwnerResp := sendWSRequest(t, collabToken, ReqRevokeQuestionSetCollaborator, map[string]any{
		"question_set_id": qs.ID.String(),
		"user_id":         collab.ID.String(),
	})
	assert.Equal(t, EvtError, nonOwnerResp.Type)
}

// ---------------------------------------------------------------------------
// TestGetQuestionSetAccess_Owner
// ---------------------------------------------------------------------------

func TestGetQuestionSetAccess_Owner(t *testing.T) {
	setup()

	owner, _, _, qs, _ := createQuestionSetTestFixture(t, "access-owner")

	hub := NewHub(db, nil, "test-secret", nil)
	mode, gotQS, _, err := hub.getQuestionSetAccess(db, owner.ID, qs.ID)
	require.NoError(t, err)
	assert.Equal(t, accessOwner, mode)
	assert.Equal(t, qs.ID, gotQS.ID)
}

// ---------------------------------------------------------------------------
// TestGetQuestionSetAccess_Editor
// ---------------------------------------------------------------------------

func TestGetQuestionSetAccess_Editor(t *testing.T) {
	setup()

	owner, _, _, qs, _ := createQuestionSetTestFixture(t, "access-editor-owner")
	collab, _, _, _, _ := createQuestionSetTestFixture(t, "access-editor-collab")

	now := time.Now()
	collaborator := models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          collab.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}
	require.NoError(t, db.Create(&collaborator).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	mode, gotQS, _, err := hub.getQuestionSetAccess(db, collab.ID, qs.ID)
	require.NoError(t, err)
	assert.Equal(t, accessEditor, mode)
	assert.Equal(t, qs.ID, gotQS.ID)
}

// ---------------------------------------------------------------------------
// TestGetQuestionSetAccess_Revoked
// ---------------------------------------------------------------------------

func TestGetQuestionSetAccess_Revoked(t *testing.T) {
	setup()

	owner, _, _, qs, _ := createQuestionSetTestFixture(t, "access-revoked-owner")
	collab, _, _, _, _ := createQuestionSetTestFixture(t, "access-revoked-collab")

	now := time.Now()
	revokedAt := now.Add(1 * time.Second)
	collaborator := models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          collab.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
		RevokedAt:       &revokedAt,
	}
	require.NoError(t, db.Create(&collaborator).Error)

	hub := NewHub(db, nil, "test-secret", nil)
	mode, _, _, err := hub.getQuestionSetAccess(db, collab.ID, qs.ID)
	require.NoError(t, err)
	assert.Equal(t, accessNone, mode)
}

// ---------------------------------------------------------------------------
// TestGetQuestionSetAccess_None
// ---------------------------------------------------------------------------

func TestGetQuestionSetAccess_None(t *testing.T) {
	setup()

	_, _, _, qs, _ := createQuestionSetTestFixture(t, "access-none-owner")
	stranger, _, _, _, _ := createQuestionSetTestFixture(t, "access-none-stranger")

	hub := NewHub(db, nil, "test-secret", nil)
	mode, _, _, err := hub.getQuestionSetAccess(db, stranger.ID, qs.ID)
	require.NoError(t, err)
	assert.Equal(t, accessNone, mode)
}

// ---------------------------------------------------------------------------
// TestGetCollabInvite_ReturnsStatus
// ---------------------------------------------------------------------------

func TestGetCollabInvite_ReturnsStatus(t *testing.T) {
	setup()

	_, _, _, qs, ownerToken := createQuestionSetTestFixture(t, "collab-get-owner")
	_, _, _, _, otherToken := createQuestionSetTestFixture(t, "collab-get-other")

	// Create invite
	createResp := sendWSRequest(t, ownerToken, ReqCreateQuestionSetCollabInvite, map[string]any{
		"question_set_id": qs.ID.String(),
	})
	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)

	// Another user can inspect it.
	getResp := sendWSRequest(t, otherToken, ReqGetQuestionSetCollabInvite, map[string]any{
		"token": created.Token,
	})
	assert.Equal(t, DataResponse, getResp.Type)

	var preview map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(getResp.Payload, &preview))
	var status string
	require.NoError(t, json.Unmarshal(preview["status"], &status))
	assert.Equal(t, "ready", status)
}

// ---------------------------------------------------------------------------
// TestBroadcastToQuestionSetAudience_ReachesOwnerAndCollaborators
// ---------------------------------------------------------------------------

func TestBroadcastToQuestionSetAudience_ReachesOwnerAndCollaborators(t *testing.T) {
	setup()

	owner, _, _, qs, _ := createQuestionSetTestFixture(t, "broadcast-owner")
	collab, _, _, _, _ := createQuestionSetTestFixture(t, "broadcast-collab")
	stranger, _, _, _, _ := createQuestionSetTestFixture(t, "broadcast-stranger")

	// Accept collaborator.
	now := time.Now()
	require.NoError(t, db.Create(&models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          collab.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}).Error)

	hub := NewHub(db, nil, "test-secret", nil)

	ownerConn := &Connection{ID: uuid.New(), UserID: owner.ID, Send: make(chan []byte, 10), Done: make(chan struct{})}
	collabConn := &Connection{ID: uuid.New(), UserID: collab.ID, Send: make(chan []byte, 10), Done: make(chan struct{})}
	strangerConn := &Connection{ID: uuid.New(), UserID: stranger.ID, Send: make(chan []byte, 10), Done: make(chan struct{})}

	// Register connections directly (same package access).
	hub.mu.Lock()
	hub.connections[ownerConn.ID] = ownerConn
	hub.connections[collabConn.ID] = collabConn
	hub.connections[strangerConn.ID] = strangerConn
	hub.mu.Unlock()

	testMsg := []byte(`{"type":"EVT_TEST","payload":"hello"}`)
	hub.BroadcastToQuestionSetAudience(qs.ID, testMsg)

	// Owner must receive.
	select {
	case msg := <-ownerConn.Send:
		assert.Equal(t, testMsg, msg)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("owner did not receive broadcast")
	}

	// Collaborator must receive.
	select {
	case msg := <-collabConn.Send:
		assert.Equal(t, testMsg, msg)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("collaborator did not receive broadcast")
	}

	// Stranger must NOT receive anything.
	select {
	case <-strangerConn.Send:
		t.Fatal("stranger should not receive broadcast")
	case <-time.After(50 * time.Millisecond):
		// expected: no message
	}
}

// ---------------------------------------------------------------------------
// TestBroadcastToQuestionSetAudience_SkipsRevoked
// ---------------------------------------------------------------------------

func TestBroadcastToQuestionSetAudience_SkipsRevoked(t *testing.T) {
	setup()

	owner, _, _, qs, _ := createQuestionSetTestFixture(t, "broadcast-revoked-owner")
	revoked, _, _, _, _ := createQuestionSetTestFixture(t, "broadcast-revoked-collab")

	// Collaborator accepted then revoked.
	now := time.Now()
	revokedAt := now.Add(time.Second)
	require.NoError(t, db.Create(&models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qs.ID,
		UserID:          revoked.ID,
		Role:            "editor",
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
		RevokedAt:       &revokedAt,
	}).Error)

	hub := NewHub(db, nil, "test-secret", nil)

	ownerConn := &Connection{ID: uuid.New(), UserID: owner.ID, Send: make(chan []byte, 10), Done: make(chan struct{})}
	revokedConn := &Connection{ID: uuid.New(), UserID: revoked.ID, Send: make(chan []byte, 10), Done: make(chan struct{})}

	hub.mu.Lock()
	hub.connections[ownerConn.ID] = ownerConn
	hub.connections[revokedConn.ID] = revokedConn
	hub.mu.Unlock()

	testMsg := []byte(`{"type":"EVT_TEST","payload":"hello"}`)
	hub.BroadcastToQuestionSetAudience(qs.ID, testMsg)

	// Owner must receive.
	select {
	case msg := <-ownerConn.Send:
		assert.Equal(t, testMsg, msg)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("owner did not receive broadcast")
	}

	// Revoked collaborator must NOT receive.
	select {
	case <-revokedConn.Send:
		t.Fatal("revoked collaborator should not receive broadcast")
	case <-time.After(50 * time.Millisecond):
		// expected: no message
	}
}
