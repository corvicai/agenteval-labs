package api

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"

	"benchmarking-platform/models"
)

// An agent whose stored config no longer decrypts (encryption key changed)
// must be rejected at run start with an actionable "re-enter credentials"
// message — not the misleading "missing credentials (API Key is required)".
func TestHandleStartRun_DecryptionFailedConfig(t *testing.T) {
	setup()

	user, token := createTestUser(t, false)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: []byte(`{}`)}
	if err := db.Create(&qs).Error; err != nil {
		t.Fatalf("create question set: %v", err)
	}

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		Name:         "BMW",
		ProviderType: "openai",
		Enabled:      true,
		Config:       models.EncryptedJSON(`{"api_key":"sk-valid"}`),
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Corrupt the stored ciphertext: valid base64, but sealed with no key we
	// have — decrypt fails, EncryptedJSON.Scan poisons the config.
	garbage := base64.StdEncoding.EncodeToString([]byte("this-is-not-a-valid-aes-gcm-ciphertext!!"))
	if err := db.Exec("UPDATE agents SET config = ? WHERE id = ?", garbage, agent.ID).Error; err != nil {
		t.Fatalf("corrupt agent config: %v", err)
	}

	resp := sendWSRequest(t, token, CmdStartRun, models.StartRunPayload{
		QuestionSetID: qs.ID.String(),
		AgentIDs:      []string{agent.ID.String()},
	})

	if resp.Type != EvtError {
		t.Fatalf("expected %s, got %s (payload: %s)", EvtError, resp.Type, string(resp.Payload))
	}
	body := string(resp.Payload)
	if !strings.Contains(body, "could not be decrypted") {
		t.Fatalf("expected decryption-failure message, got: %s", body)
	}
	if !strings.Contains(body, "BMW") {
		t.Fatalf("expected agent name in message, got: %s", body)
	}
}
