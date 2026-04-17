package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// --- WebSocket Client (copied from reset) ---

type WSClient struct {
	conn           *websocket.Conn
	responses      map[string]chan Response
	mu             sync.Mutex
	writeMu        sync.Mutex
	done           chan struct{}
	token          string
	workspaceID    string
	organizationID string
}

type Response struct {
	Type    string
	Payload json.RawMessage
}

type Envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

func NewWSClient(wsURL string) (*WSClient, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}

	client := &WSClient{
		conn:      conn,
		responses: make(map[string]chan Response),
		done:      make(chan struct{}),
	}

	go client.readLoop()
	return client, nil
}

func (c *WSClient) readLoop() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			close(c.done)
			return
		}

		var env Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			continue
		}

		c.mu.Lock()
		if ch, ok := c.responses[env.CorrelationID]; ok {
			ch <- Response{Type: env.Type, Payload: env.Payload}
			delete(c.responses, env.CorrelationID)
		}
		c.mu.Unlock()
	}
}

func (c *WSClient) Send(msgType string, payload any) (json.RawMessage, error) {
	corrID := uuid.New().String()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	env := Envelope{
		Type:          msgType,
		CorrelationID: corrID,
		Payload:       payloadBytes,
	}

	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	respChan := make(chan Response, 1)
	c.mu.Lock()
	c.responses[corrID] = respChan
	c.mu.Unlock()

	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, envBytes)
	c.writeMu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.responses, corrID)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respChan:
		if resp.Type == "EVT_ERROR" {
			var errPayload struct {
				Error   string `json:"error"`
				Details any    `json:"details"`
			}
			if err := json.Unmarshal(resp.Payload, &errPayload); err != nil {
				return nil, fmt.Errorf("server returned EVT_ERROR with unparseable payload: %w", err)
			}
			if errPayload.Details != nil {
				return nil, fmt.Errorf("%s (details: %v)", errPayload.Error, errPayload.Details)
			}
			return nil, fmt.Errorf("%s", errPayload.Error)
		}
		return resp.Payload, nil
	case <-time.After(60 * time.Second):
		c.mu.Lock()
		delete(c.responses, corrID)
		c.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for response")
	case <-c.done:
		return nil, fmt.Errorf("connection closed")
	}
}

func (c *WSClient) Close() {
	c.conn.Close()
}

// --- Test Logic ---

func main() {
	wsURL := flag.String("ws", "ws://localhost:3010/ws", "WebSocket URL")
	flag.Parse()

	log.Println("🧪 Agent Test Suite Started")
	log.Printf("   WebSocket: %s", *wsURL)

	// Connect
	client, err := NewWSClient(*wsURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer client.Close()
	log.Println("✅ Connected to WebSocket")

	// Step 1: Create temporary admin user via Bootstrap
	log.Println("\n📋 Step 1: Bootstrapping temporary admin user...")
	timestamp := time.Now().UnixNano()
	tmpEmail := fmt.Sprintf("test_%d@tmp.local", timestamp)
	tmpPass := "testpass123"
	tmpOrg := fmt.Sprintf("TMP Org %d", timestamp)

	_, err = client.Send("REQ_WS_BOOTSTRAP_ADMIN", map[string]any{
		"name":              "Test Admin",
		"email":             tmpEmail,
		"password":          tmpPass,
		"organization_name": tmpOrg,
	})
	if err != nil {
		log.Fatalf("❌ Failed to bootstrap: %v", err)
	}
	log.Println("✅ User bootstrapped")

	// Login
	log.Println("📋 Logging in...")
	loginResp, err := client.Send("REQ_WS_LOGIN", map[string]any{
		"email":    tmpEmail,
		"password": tmpPass,
	})
	if err != nil {
		log.Fatalf("❌ Failed to login: %v", err)
	}

	var loginData struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
		Organization struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organization"`
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(loginResp, &loginData); err != nil {
		log.Fatalf("❌ Failed to parse login response: %v", err)
	}
	client.token = loginData.Token
	client.organizationID = loginData.Organization.ID
	client.workspaceID = loginData.Workspace.ID
	log.Printf("✅ Logged in (org: %s, default ws: %s)", client.organizationID, client.workspaceID)

	// Create workspace "TMP"
	log.Println("\n📋 Step 2: Creating workspace TMP...")
	wsResp, err := client.Send("REQ_CREATE_WORKSPACE", map[string]any{
		"name": "TMP",
	})
	if err != nil {
		log.Fatalf("❌ Failed to create workspace: %v", err)
	}

	var wsData struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(wsResp, &wsData); err != nil {
		log.Fatalf("❌ Failed to parse workspace response: %v", err)
	}
	client.workspaceID = wsData.ID
	log.Printf("✅ Workspace created: %s", client.workspaceID)

	// Switch to workspace (redundant but good to verify)
	_, err = client.Send("REQ_SWITCH_WORKSPACE", map[string]any{
		"workspace_id": client.workspaceID,
	})
	if err != nil {
		log.Fatalf("❌ Failed to switch workspace: %v", err)
	}
	log.Println("✅ Switched to workspace TMP")

	// Step 3: Create 3 mock agents
	log.Println("\n📋 Step 3: Creating 3 mock agents...")
	agentIDs := []string{}
	for i := 1; i <= 3; i++ {
		agentResp, err := client.Send("REQ_CREATE_AGENT", map[string]any{
			"workspace_id":  client.workspaceID, // Added workspace_id
			"name":          fmt.Sprintf("Mock Agent %d", i),
			"provider_type": "mcp",
			"config": map[string]any{
				"mock_mode": true,
				"endpoint":  "mock://localhost",
			},
		})
		if err != nil {
			log.Fatalf("❌ Failed to create agent %d: %v", i, err)
		}

		var agentData struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(agentResp, &agentData); err != nil {
			log.Fatalf("❌ Failed to parse agent %d response: %v", i, err)
		}
		agentIDs = append(agentIDs, agentData.ID)
		log.Printf("   ✅ Created agent %d: %s", i, agentData.ID)
	}

	// Step 4: Create question set with 3 questions
	log.Println("\n📋 Step 4: Creating question set with 3 questions...")
	qsResp, err := client.Send("REQ_CREATE_QUESTION_SET", map[string]any{
		"workspace_id": client.workspaceID, // Added workspace_id
		"name":         "Test QS 1",
		"data": map[string]any{
			"questions": []map[string]any{
				{"id": "q1", "question": "What is 2+2?"},
				{"id": "q2", "question": "What is the capital of France?"},
				{"id": "q3", "question": "What color is the sky?"},
			},
		},
	})
	if err != nil {
		log.Fatalf("❌ Failed to create question set: %v", err)
	}

	var qsData struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(qsResp, &qsData); err != nil {
		log.Fatalf("❌ Failed to parse question set response: %v", err)
	}
	qs1ID := qsData.ID
	log.Printf("✅ Question Set created: %s", qs1ID)

	// Assign all 3 agents to the question set
	log.Println("\n📋 Step 5: Assigning agents to question set...")
	qsAgents := []map[string]any{}
	for i, aid := range agentIDs {
		qsAgents = append(qsAgents, map[string]any{
			"agent_id": aid,
			"enabled":  true,
			"position": i,
			"config":   map[string]any{},
		})
	}

	_, err = client.Send("REQ_UPDATE_QUESTION_SET_AGENTS", map[string]any{
		"question_set_id": qs1ID,
		"agents":          qsAgents,
	})
	if err != nil {
		log.Fatalf("❌ Failed to assign agents: %v", err)
	}
	log.Println("✅ 3 agents assigned to question set")

	// Step 6: Remove one agent (disable it)
	log.Println("\n📋 Step 6: Removing 1 agent (disabling)...")
	qsAgents[2]["enabled"] = false // Disable the 3rd agent
	_, err = client.Send("REQ_UPDATE_QUESTION_SET_AGENTS", map[string]any{
		"question_set_id": qs1ID,
		"agents":          qsAgents,
	})
	if err != nil {
		log.Fatalf("❌ Failed to update agents: %v", err)
	}
	log.Println("✅ Agent 3 disabled")

	// Step 7: Verify only 2 agents are enabled
	log.Println("\n📋 Step 7: Verifying agent count...")
	syncResp, err := client.Send("REQ_SYNC_STATE", map[string]any{})
	if err != nil {
		log.Fatalf("❌ Failed to sync state: %v", err)
	}

	var syncData struct {
		QuestionSets []struct {
			ID     string `json:"id"`
			Agents []struct {
				AgentID string `json:"agent_id"`
				Enabled bool   `json:"enabled"`
			} `json:"agents"`
		} `json:"question_sets"`
	}
	if err := json.Unmarshal(syncResp, &syncData); err != nil {
		log.Fatalf("❌ Failed to parse sync state response: %v", err)
	}

	enabledCount := 0
	foundQS := false
	for _, qs := range syncData.QuestionSets {
		if qs.ID == qs1ID {
			foundQS = true
			for _, a := range qs.Agents {
				if a.Enabled {
					enabledCount++
				}
			}
		}
	}

	if !foundQS {
		log.Fatalf("❌ FAIL: Question Set %s not found in sync state", qs1ID)
	}

	if enabledCount == 2 {
		log.Println("✅ PASS: 2 agents enabled as expected")
	} else {
		log.Fatalf("❌ FAIL: Expected 2 enabled agents, got %d", enabledCount)
	}

	// Step 8: Create 4th agent
	log.Println("\n📋 Step 8: Creating 4th agent...")
	agent4Resp, err := client.Send("REQ_CREATE_AGENT", map[string]any{
		"workspace_id":  client.workspaceID, // Added workspace_id
		"name":          "Mock Agent 4",
		"provider_type": "mcp",
		"config": map[string]any{
			"mock_mode": true,
			"endpoint":  "mock://localhost",
		},
	})
	if err != nil {
		log.Fatalf("❌ Failed to create agent 4: %v", err)
	}
	var agent4Data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(agent4Resp, &agent4Data); err != nil {
		log.Fatalf("❌ Failed to parse agent 4 response: %v", err)
	}
	agentIDs = append(agentIDs, agent4Data.ID)
	log.Printf("✅ Created agent 4: %s", agent4Data.ID)

	// Step 9: Create 2nd question set with 2 questions and 4 agents
	log.Println("\n📋 Step 9: Creating 2nd question set with 2 questions...")
	qs2Resp, err := client.Send("REQ_CREATE_QUESTION_SET", map[string]any{
		"workspace_id": client.workspaceID, // Added workspace_id
		"name":         "Test QS 2",
		"data": map[string]any{
			"questions": []map[string]any{
				{"id": "q1", "question": "Is the earth round?"},
				{"id": "q2", "question": "What is 10 x 10?"},
			},
		},
	})
	if err != nil {
		log.Fatalf("❌ Failed to create question set 2: %v", err)
	}

	var qs2Data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(qs2Resp, &qs2Data); err != nil {
		log.Fatalf("❌ Failed to parse question set 2 response: %v", err)
	}
	qs2ID := qs2Data.ID
	log.Printf("✅ Question Set 2 created: %s", qs2ID)

	// Assign all 4 agents
	log.Println("\n📋 Step 10: Assigning 4 agents to question set 2...")
	qs2Agents := []map[string]any{}
	for i, aid := range agentIDs {
		qs2Agents = append(qs2Agents, map[string]any{
			"agent_id": aid,
			"enabled":  true,
			"position": i,
			"config":   map[string]any{},
		})
	}

	_, err = client.Send("REQ_UPDATE_QUESTION_SET_AGENTS", map[string]any{
		"question_set_id": qs2ID,
		"agents":          qs2Agents,
	})
	if err != nil {
		log.Fatalf("❌ Failed to assign agents to QS2: %v", err)
	}
	log.Println("✅ 4 agents assigned to question set 2")

	// Step 11: Remove 3 agents (keep only 1)
	log.Println("\n📋 Step 11: Removing 3 agents (keeping 1)...")
	// Keep agent 0, disable 1, 2, 3
	for i := 1; i < 4; i++ {
		qs2Agents[i]["enabled"] = false
	}

	_, err = client.Send("REQ_UPDATE_QUESTION_SET_AGENTS", map[string]any{
		"question_set_id": qs2ID,
		"agents":          qs2Agents,
	})
	if err != nil {
		log.Fatalf("❌ Failed to update agents for QS2: %v", err)
	}
	log.Println("✅ 3 agents disabled")

	// Step 12: Verify only 1 agent is enabled
	log.Println("\n📋 Step 12: Verifying final agent count...")
	syncResp2, err := client.Send("REQ_SYNC_STATE", map[string]any{})
	if err != nil {
		log.Fatalf("❌ Failed to sync state: %v", err)
	}

	var syncData2 struct {
		QuestionSets []struct {
			ID     string `json:"id"`
			Agents []struct {
				AgentID string `json:"agent_id"`
				Enabled bool   `json:"enabled"`
			} `json:"agents"`
		} `json:"question_sets"`
	}
	if err := json.Unmarshal(syncResp2, &syncData2); err != nil {
		log.Fatalf("❌ Failed to parse sync state 2 response: %v", err)
	}

	qs2EnabledCount := 0
	foundQS2 := false
	for _, qs := range syncData2.QuestionSets {
		if qs.ID == qs2ID {
			foundQS2 = true
			for _, a := range qs.Agents {
				if a.Enabled {
					qs2EnabledCount++
				}
			}
		}
	}

	if !foundQS2 {
		log.Fatalf("❌ FAIL: Question Set 2 %s not found in sync state", qs2ID)
	}

	if qs2EnabledCount == 1 {
		log.Println("✅ PASS: 1 agent enabled as expected")
	} else {
		log.Fatalf("❌ FAIL: Expected 1 enabled agent, got %d", qs2EnabledCount)
	}

	// Cleanup (optional - delete workspace/user)
	log.Println("\n🎉 ALL TESTS PASSED!")
	log.Println("   Cleanup: Temporary data will remain in DB.")
	log.Println("   Run reset.sh to clean up.")
}
