package api

import (
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

func setup() {
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Migrate the schema
	db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.UserOrganization{},
		&models.Workspace{},
		&models.InviteCode{},
	)
}

func createTestUser(t *testing.T, isAdmin bool) (models.User, string) {
	user := models.User{
		ID:           uuid.New(),
		Name:         "Test User",
		Email:        "test@example.com",
		PasswordHash: "hash",
		IsAdmin:      isAdmin,
	}
	db.Create(&user)

	// Admin needs org and workspace too for token generation usually, or at least dummy ones
	org := models.Organization{
		ID:   uuid.New(),
		Name: "Test Org",
	}
	db.Create(&org)

	return user, generateTestToken(user.ID, uuid.Nil, org.ID)
}

func generateTestToken(userID, workspaceID, orgID uuid.UUID) string {
	os.Setenv("JWT_SECRET", "test-secret")
	token, _ := middleware.GenerateToken(
		userID.String(),
		workspaceID.String(),
		orgID.String(),
		"test@example.com",
		"test-secret",
		"",
	)
	return token
}

// Mocking WS interaction with direct handler calls or simulated structure
// Since we can't easily spin up a full WS server in unit tests without complex setup,
// we will simulate the Hub handling logic or use an integration test approach.
// However, for this task, I'll mock the Hub's dependency on DB and call handlers directly if possible,
// OR (simpler) just test the logic via a simulated Hub instance.

func sendWSRequest(t *testing.T, token string, msgType string, payload any) *models.Envelope {
	// Create a temporary Hub with our test DB
	hub := NewHub(db, nil) // Engine nil is fine for these tests

	// Create a mock connection
	conn := &Connection{
		ID:   uuid.New(),
		Send: make(chan []byte, 100),
	}

	// Parse token to populate conn (simulating auth middleware/handshake)
	if token != "" {
		tokenFn := func(token *jwt.Token) (any, error) {
			return []byte("test-secret"), nil
		}

		parsedToken, _ := jwt.Parse(token, tokenFn)

		if parsedToken != nil && parsedToken.Valid {
			if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(string); ok {
					conn.UserID, _ = uuid.Parse(uid)
				}
				if orgId, ok := claims["org_id"].(string); ok {
					conn.OrgID, _ = uuid.Parse(orgId)
				}
				conn.IsAuthenticated = true
			}
		}
	}

	// Prepare request
	payloadBytes, _ := json.Marshal(payload)
	env := models.Envelope{
		Type:          msgType,
		CorrelationID: "test-req",
		Payload:       payloadBytes,
	}

	// Dispatch based on Type manual routing for tests
	switch env.Type {
	case ReqAdminGenerateInvite:
		hub.handleAdminGenerateInvite(conn, env)
	case ReqManagerGenerateInvite:
		hub.handleManagerGenerateInvite(conn, env)
	case ReqWsRegister:
		hub.handleWsRegister(conn, env)
	default:
		// Fallback or ignore for now
	}

	// Capture response from channel
	select {
	case respBytes := <-conn.Send:
		var resp models.Envelope
		json.Unmarshal(respBytes, &resp)
		return &resp
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
		return nil
	}
}
