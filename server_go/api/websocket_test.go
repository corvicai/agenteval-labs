package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_Registration(t *testing.T) {
	hub := NewHub(nil, nil, "test-secret", nil)
	go hub.Run()

	t.Run("registers and unregisters connections", func(t *testing.T) {
		conn := &Connection{
			ID:          uuid.New(),
			WorkspaceID: uuid.New(),
			Send:        make(chan []byte, 256),
			Done:        make(chan struct{}),
		}

		hub.Register(conn)
		time.Sleep(50 * time.Millisecond)

		hub.mu.RLock()
		_, exists := hub.connections[conn.ID]
		hub.mu.RUnlock()
		assert.True(t, exists)

		hub.Unregister(conn)
		time.Sleep(50 * time.Millisecond)

		hub.mu.RLock()
		_, exists = hub.connections[conn.ID]
		hub.mu.RUnlock()
		assert.False(t, exists)
	})
}

func TestHub_BroadcastToWorkspace(t *testing.T) {
	hub := NewHub(nil, nil, "test-secret", nil)
	go hub.Run()

	workspaceA := uuid.New()
	workspaceB := uuid.New()

	connA1 := &Connection{ID: uuid.New(), WorkspaceID: workspaceA, Send: make(chan []byte, 256), Done: make(chan struct{})}
	connA2 := &Connection{ID: uuid.New(), WorkspaceID: workspaceA, Send: make(chan []byte, 256), Done: make(chan struct{})}
	connB := &Connection{ID: uuid.New(), WorkspaceID: workspaceB, Send: make(chan []byte, 256), Done: make(chan struct{})}

	hub.Register(connA1)
	hub.Register(connA2)
	hub.Register(connB)
	time.Sleep(50 * time.Millisecond)

	t.Run("broadcasts only to matching workspace", func(t *testing.T) {
		msg := []byte(`{"type":"test"}`)
		hub.BroadcastToWorkspace(workspaceA, msg)

		// WorkspaceA connections should receive
		select {
		case received := <-connA1.Send:
			assert.Equal(t, msg, received)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("connA1 should have received message")
		}

		select {
		case received := <-connA2.Send:
			assert.Equal(t, msg, received)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("connA2 should have received message")
		}

		// WorkspaceB should NOT receive
		select {
		case <-connB.Send:
			t.Fatal("connB should NOT have received message")
		case <-time.After(100 * time.Millisecond):
			// Expected - no message
		}
	})
}

func TestHub_SendEvent(t *testing.T) {
	hub := NewHub(nil, nil, "test-secret", nil)
	go hub.Run()

	workspaceID := uuid.New()
	conn := &Connection{ID: uuid.New(), WorkspaceID: workspaceID, Send: make(chan []byte, 256), Done: make(chan struct{})}
	hub.Register(conn)
	time.Sleep(50 * time.Millisecond)

	t.Run("sends properly formatted event", func(t *testing.T) {
		payload := models.TaskCompletedPayload{
			RunID:      uuid.New().String(),
			AgentID:    uuid.New().String(),
			QuestionID: "q-1",
			Success:    true,
			Answer:     "Test answer",
			DurationMs: 150,
		}

		err := hub.SendEvent(workspaceID, EvtTaskCompleted, "corr-123", payload)
		assert.NoError(t, err)

		select {
		case msg := <-conn.Send:
			var env models.Envelope
			err := json.Unmarshal(msg, &env)
			assert.NoError(t, err)
			assert.Equal(t, EvtTaskCompleted, env.Type)
			assert.Equal(t, "corr-123", env.CorrelationID)

			var receivedPayload models.TaskCompletedPayload
			json.Unmarshal(env.Payload, &receivedPayload)
			assert.Equal(t, "q-1", receivedPayload.QuestionID)
			assert.True(t, receivedPayload.Success)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Should have received event")
		}
	})
}

func TestWebSocket_Integration(t *testing.T) {
	hub := NewHub(nil, nil, "test-secret", nil)
	go hub.Run()

	// Create test server
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		workspaceIDStr := r.URL.Query().Get("workspace_id")
		workspaceID, _ := uuid.Parse(workspaceIDStr)

		conn := &Connection{
			ID:          uuid.New(),
			WorkspaceID: workspaceID,
			Conn:        ws,
			Send:        make(chan []byte, 256),
			Done:        make(chan struct{}),
		}

		hub.Register(conn)
		go conn.WritePump()

		// Read messages
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				break
			}

			var env models.Envelope
			if err := json.Unmarshal(message, &env); err != nil {
				continue
			}

			// Echo back with acknowledgment
			ack := models.Envelope{
				Type:          "ACK_" + env.Type,
				CorrelationID: env.CorrelationID,
			}
			ackBytes, _ := json.Marshal(ack)
			conn.Send <- ackBytes
		}

		hub.Unregister(conn)
	}))
	defer server.Close()

	t.Run("client can connect and exchange messages", func(t *testing.T) {
		workspaceID := uuid.New()
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?workspace_id=" + workspaceID.String()

		dialer := websocket.Dialer{}
		ws, _, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer ws.Close()

		// Send command
		cmd := models.Envelope{
			Type:          CmdStartRun,
			CorrelationID: "test-corr-1",
		}
		cmdBytes, _ := json.Marshal(cmd)
		err = ws.WriteMessage(websocket.TextMessage, cmdBytes)
		assert.NoError(t, err)

		// Receive acknowledgment
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)

		var ack models.Envelope
		json.Unmarshal(message, &ack)
		assert.Equal(t, "ACK_CMD_START_RUN", ack.Type)
		assert.Equal(t, "test-corr-1", ack.CorrelationID)
	})

	t.Run("multiple clients in same workspace receive broadcasts", func(t *testing.T) {
		workspaceID := uuid.New()
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?workspace_id=" + workspaceID.String()

		dialer := websocket.Dialer{}

		// Connect 3 clients
		var clients []*websocket.Conn
		for i := 0; i < 3; i++ {
			ws, _, err := dialer.Dial(wsURL, nil)
			require.NoError(t, err)
			clients = append(clients, ws)
		}
		defer func() {
			for _, c := range clients {
				c.Close()
			}
		}()

		time.Sleep(100 * time.Millisecond) // Wait for registration

		// Broadcast to workspace
		hub.BroadcastToWorkspace(workspaceID, []byte(`{"test":"broadcast"}`))

		// All clients should receive
		var wg sync.WaitGroup
		received := make([]bool, 3)

		for i, client := range clients {
			wg.Add(1)
			go func(idx int, c *websocket.Conn) {
				defer wg.Done()
				c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _, err := c.ReadMessage()
				if err == nil {
					received[idx] = true
				}
			}(i, client)
		}

		wg.Wait()

		for i, r := range received {
			assert.True(t, r, "Client %d should have received broadcast", i)
		}
	})
}

func TestEnvelope_Parsing(t *testing.T) {
	t.Run("parses valid models.models.Envelope", func(t *testing.T) {
		raw := `{"type":"CMD_START_RUN","correlation_id":"abc-123","payload":{"question_set_id":"qs-1"}}`

		var env models.Envelope
		err := json.Unmarshal([]byte(raw), &env)
		assert.NoError(t, err)
		assert.Equal(t, CmdStartRun, env.Type)
		assert.Equal(t, "abc-123", env.CorrelationID)

		var payload models.StartRunPayload
		json.Unmarshal(env.Payload, &payload)
		assert.Equal(t, "qs-1", payload.QuestionSetID)
	})

	t.Run("handles missing fields gracefully", func(t *testing.T) {
		raw := `{"type":"CMD_CANCEL_RUN"}`

		var env models.Envelope
		err := json.Unmarshal([]byte(raw), &env)
		assert.NoError(t, err)
		assert.Equal(t, CmdCancelRun, env.Type)
		assert.Empty(t, env.CorrelationID)
	})
}
