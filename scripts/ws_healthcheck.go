package scripts

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

func ws_healthcheck() {
	wsURL := os.Getenv("WS_URL")
	email := os.Getenv("ADMIN_EMAIL")
	pass := os.Getenv("ADMIN_PASS")
	iapIDToken := os.Getenv("IAP_ID_TOKEN")

	if wsURL == "" || email == "" || pass == "" {
		log.Fatal("WS_URL, ADMIN_EMAIL, and ADMIN_PASS are required")
	}

	dialer := websocket.Dialer{}
	headers := http.Header{}

	if iapIDToken != "" {
		headers.Set("Authorization", "Bearer "+iapIDToken)
		// Keep parity with browser flow when IAP expects WS subprotocol token.
		dialer.Subprotocols = []string{"iap-bearer-token", iapIDToken}
		log.Println("IAP token provided; dialing WS with Authorization header and iap-bearer-token subprotocol")
	}

	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		log.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	send := func(msgType string, payload any) ([]byte, error) {
		corr := uuid.NewString()
		p, _ := json.Marshal(payload)
		envBytes, _ := json.Marshal(envelope{Type: msgType, CorrelationID: corr, Payload: p})
		if err := conn.WriteMessage(websocket.TextMessage, envBytes); err != nil {
			return nil, err
		}
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return nil, err
			}
			var envResp envelope
			if json.Unmarshal(msg, &envResp) == nil && envResp.CorrelationID == corr {
				return envResp.Payload, nil
			}
		}
	}

	// Login
	if _, err := send("REQ_WS_LOGIN", map[string]string{"email": email, "password": pass}); err != nil {
		log.Fatalf("login failed: %v", err)
	}
	log.Println("login ok")

	// DB perf read
	if resp, err := send("REQ_CHECK_DB_PERF", nil); err == nil {
		log.Printf("db perf: %s", resp)
	} else {
		log.Printf("db perf error: %v", err)
	}

	// Create org (write)
	orgName := "health-org-" + uuid.NewString()[0:8]
	start := time.Now()
	if _, err := send("REQ_ADMIN_CREATE_ORG", map[string]string{"name": orgName}); err != nil {
		log.Fatalf("create org failed: %v", err)
	}
	log.Printf("org %s created in %v", orgName, time.Since(start))
}
