package api

import (
	"benchmarking-platform/models"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func setup() {
	var err error
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second,   // Slow SQL threshold
			LogLevel:                  logger.Silent, // Log level
			IgnoreRecordNotFoundError: true,          // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      true,          // Don't include params in the SQL log
			Colorful:                  false,         // Disable color
		},
	)

	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: newLogger,
	})
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
		&models.InviteCodeUsage{},
		&models.Passkey{},
		&models.AuditLog{},
		&models.LoginLog{},
		&models.StatsCache{},
		&models.Agent{},
		&models.Client{},
		&models.QuestionSet{},
		&models.QuestionSetAgent{},
		&models.Run{},
		&models.RunResult{},
		&models.Evaluation{},
	)
}

func createTestUser(t *testing.T, isAdmin bool) (models.User, string) {
	uniqueID := uuid.New().String()
	user := models.User{
		ID:           uuid.New(),
		Name:         "Test User " + uniqueID,
		Email:        "test_" + uniqueID + "@example.com",
		PasswordHash: "hash",
		IsAdmin:      isAdmin,
	}
	db.Create(&user)

	// Admin needs org and workspace too for token generation usually, or at least dummy ones
	org := models.Organization{
		ID:   uuid.New(),
		Name: "Test Org " + uniqueID,
	}
	db.Create(&org)

	// Link user to org
	db.Create(&models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Role:           "manager",
		JoinedAt:       time.Now(),
	})

	return user, generateTestToken(user.ID, uuid.Nil, org.ID)
}

func generateTestToken(userID, workspaceID, orgID uuid.UUID) string {
	os.Setenv("JWT_SECRET", "test-secret")
	claims := jwt.MapClaims{
		"user_id":      userID.String(),
		"workspace_id": workspaceID.String(),
		"org_id":       orgID.String(),
		"email":        "test@example.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("test-secret"))
	return signed
}

// Mocking WS interaction with direct handler calls or simulated structure
// Since we can't easily spin up a full WS server in unit tests without complex setup,
// we will simulate the Hub handling logic or use an integration test approach.
// However, for this task, I'll mock the Hub's dependency on DB and call handlers directly if possible,
// OR (simpler) just test the logic via a simulated Hub instance.

func sendWSRequest(t *testing.T, token string, msgType string, payload any) *models.Envelope {
	// Create a temporary Hub with our test DB
	hub := NewHub(db, nil, "test-secret", nil) // Engine nil is fine for these tests

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

	// Dispatch using central handler
	hub.HandleWSMessage(conn, env)

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
