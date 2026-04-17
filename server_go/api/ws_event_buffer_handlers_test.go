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

// drainMissedResponse waits for a DATA_MISSED_EVENTS response on conn.Send
// with the given correlation id, skipping unrelated envelopes.
func drainMissedResponse(t *testing.T, conn *Connection, correlationID string) models.MissedEventsResponse {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case msg := <-conn.Send:
			var env models.Envelope
			require.NoError(t, json.Unmarshal(msg, &env))
			if env.Type != DataMissedEvents || env.CorrelationID != correlationID {
				continue
			}
			var resp models.MissedEventsResponse
			require.NoError(t, json.Unmarshal(env.Payload, &resp))
			return resp
		case <-deadline:
			t.Fatalf("did not receive DATA_MISSED_EVENTS for correlation %s", correlationID)
			return models.MissedEventsResponse{}
		}
	}
}

func TestHandleGetMissedEvents_ReplaysEventsAfterCursor(t *testing.T) {
	setup()
	owner, ownerWorkspace, _, qs, _ := createQuestionSetTestFixture(t, "missed-owner")

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	// Emit a few events. The first becomes our cursor; the remaining three
	// are what the client expects to replay.
	require.NoError(t, hub.SendEventToQS(qs.ID, EvtTaskStarted, "c1", map[string]any{"k": 1}))
	first := hub.eventBuffer.events[len(hub.eventBuffer.events)-1].EventID

	require.NoError(t, hub.SendEventToQS(qs.ID, EvtTaskCompleted, "c2", map[string]any{"k": 2}))
	require.NoError(t, hub.SendEvent(ownerWorkspace.ID, EvtDataChanged, "c3", map[string]any{"k": 3}))
	require.NoError(t, hub.SendEventToUser(owner.ID, EvtCollaboratorRevoked, "c4", map[string]any{"k": 4}))

	// Register a connection that belongs to the owner / owner workspace so
	// every audience filter should match.
	conn := &Connection{
		ID:              uuid.New(),
		UserID:          owner.ID,
		WorkspaceID:     ownerWorkspace.ID,
		Send:            make(chan []byte, 16),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	hub.Register(conn)
	time.Sleep(30 * time.Millisecond)

	envelope := models.Envelope{
		Type:          ReqGetMissedEvents,
		CorrelationID: "missed-1",
	}
	payload, _ := json.Marshal(models.GetMissedEventsPayload{SinceEventID: first})
	envelope.Payload = payload

	hub.handleGetMissedEvents(conn, envelope)

	resp := drainMissedResponse(t, conn, "missed-1")
	assert.False(t, resp.NeedsFullSync, "expected in-buffer replay, got full sync")
	assert.Len(t, resp.Events, 3, "expected three replayed events")

	// Replayed envelopes should be verbatim — same type + correlation as the
	// original broadcast so the frontend can route them through the normal
	// message handler without special-casing.
	var replayed []models.Envelope
	for _, raw := range resp.Events {
		var e models.Envelope
		require.NoError(t, json.Unmarshal(raw, &e))
		replayed = append(replayed, e)
	}
	assert.Equal(t, EvtTaskCompleted, replayed[0].Type)
	assert.Equal(t, "c2", replayed[0].CorrelationID)
	assert.Equal(t, EvtDataChanged, replayed[1].Type)
	assert.Equal(t, EvtCollaboratorRevoked, replayed[2].Type)
	assert.NotEmpty(t, resp.LastEventID)
	assert.Equal(t, replayed[2].EventID, resp.LastEventID)
}

func TestHandleGetMissedEvents_UnknownNonceTriggersFullSync(t *testing.T) {
	setup()
	owner, ownerWorkspace, _, _, _ := createQuestionSetTestFixture(t, "missed-resync")

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	conn := &Connection{
		ID:              uuid.New(),
		UserID:          owner.ID,
		WorkspaceID:     ownerWorkspace.ID,
		Send:            make(chan []byte, 4),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	hub.Register(conn)
	time.Sleep(30 * time.Millisecond)

	envelope := models.Envelope{
		Type:          ReqGetMissedEvents,
		CorrelationID: "resync-1",
	}
	payload, _ := json.Marshal(models.GetMissedEventsPayload{SinceEventID: "deadbeef:5"})
	envelope.Payload = payload

	hub.handleGetMissedEvents(conn, envelope)

	resp := drainMissedResponse(t, conn, "resync-1")
	assert.True(t, resp.NeedsFullSync, "unknown nonce should force a full resync")
	assert.Empty(t, resp.Events)
}

func TestHandleGetMissedEvents_FiltersOutEventsForOtherUsers(t *testing.T) {
	setup()
	_, aliceWorkspace, _, aliceQS, _ := createQuestionSetTestFixture(t, "missed-alice")
	bob, bobWorkspace, _, _, _ := createQuestionSetTestFixture(t, "missed-bob")

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	// Alice's QS event — Bob must not receive it on replay since he is
	// neither the owner nor a collaborator.
	require.NoError(t, hub.SendEventToQS(aliceQS.ID, EvtTaskStarted, "ca1", map[string]any{}))
	cursor := hub.eventBuffer.events[0].EventID

	require.NoError(t, hub.SendEventToQS(aliceQS.ID, EvtTaskCompleted, "ca2", map[string]any{}))
	require.NoError(t, hub.SendEvent(aliceWorkspace.ID, EvtDataChanged, "ca3", map[string]any{}))
	require.NoError(t, hub.SendEventToUser(bob.ID, EvtCollaboratorRevoked, "cb1", map[string]any{}))

	bobConn := &Connection{
		ID:              uuid.New(),
		UserID:          bob.ID,
		WorkspaceID:     bobWorkspace.ID,
		Send:            make(chan []byte, 8),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	hub.Register(bobConn)
	time.Sleep(30 * time.Millisecond)

	envelope := models.Envelope{Type: ReqGetMissedEvents, CorrelationID: "bob-1"}
	payload, _ := json.Marshal(models.GetMissedEventsPayload{SinceEventID: cursor})
	envelope.Payload = payload

	hub.handleGetMissedEvents(bobConn, envelope)
	resp := drainMissedResponse(t, bobConn, "bob-1")

	assert.False(t, resp.NeedsFullSync)
	assert.Len(t, resp.Events, 1, "Bob should only see his user-targeted event")

	var replayed models.Envelope
	require.NoError(t, json.Unmarshal(resp.Events[0], &replayed))
	assert.Equal(t, EvtCollaboratorRevoked, replayed.Type)
	assert.Equal(t, "cb1", replayed.CorrelationID)
}

func TestHandleGetMissedEvents_EmptyCursorForcesFullSync(t *testing.T) {
	setup()
	owner, ownerWorkspace, _, _, _ := createQuestionSetTestFixture(t, "missed-empty")

	hub := NewHub(db, nil, "test-secret", nil)
	go hub.Run()

	conn := &Connection{
		ID:              uuid.New(),
		UserID:          owner.ID,
		WorkspaceID:     ownerWorkspace.ID,
		Send:            make(chan []byte, 4),
		Done:            make(chan struct{}),
		IsAuthenticated: true,
	}
	hub.Register(conn)
	time.Sleep(30 * time.Millisecond)

	envelope := models.Envelope{Type: ReqGetMissedEvents, CorrelationID: "empty-1"}
	payload, _ := json.Marshal(models.GetMissedEventsPayload{SinceEventID: ""})
	envelope.Payload = payload

	hub.handleGetMissedEvents(conn, envelope)
	resp := drainMissedResponse(t, conn, "empty-1")
	assert.True(t, resp.NeedsFullSync, "empty cursor must trigger full sync")
}
