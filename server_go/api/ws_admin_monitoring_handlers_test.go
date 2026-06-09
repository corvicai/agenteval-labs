package api

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"benchmarking-platform/models"
)

// Admins have no Postgres access in production; the only window into why a run
// failed is the debug snapshot. It must surface the raw RunResult errors.
func TestAdminDebugInfo_IncludesRecentRunErrors(t *testing.T) {
	setup()

	admin, token := createTestUser(t, true)

	ws := models.Workspace{ID: uuid.New(), UserID: admin.ID, Name: "WS"}
	if err := db.Create(&ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  ws.ID,
		Name:         "BMW",
		ProviderType: "openai",
		Enabled:      true,
		Config:       models.EncryptedJSON(`{"api_key":"x"}`),
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	run := models.Run{ID: uuid.New(), WorkspaceID: ws.ID, QuestionSetID: uuid.New(), Status: "completed_with_errors", TotalTasks: 1}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	rawErr := "openai request failed with status 502: <html>Bad Gateway from proxy</html>"
	rr := models.RunResult{
		ID: uuid.New(), RunID: run.ID, AgentID: agent.ID, QuestionID: "q-1",
		Status: "error", Error: rawErr, DurationMs: 1234,
	}
	if err := db.Create(&rr).Error; err != nil {
		t.Fatalf("create run result: %v", err)
	}

	resp := sendWSRequest(t, token, ReqAdminGetDebugInfo, models.AdminRunsPayload{})

	if resp.Type != DataAdminDebugInfo {
		t.Fatalf("expected %s, got %s (payload: %s)", DataAdminDebugInfo, resp.Type, string(resp.Payload))
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Payload, &body); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	errsRaw, ok := body["recent_run_errors"].([]any)
	if !ok || len(errsRaw) == 0 {
		t.Fatalf("expected recent_run_errors in payload, got: %s", string(resp.Payload))
	}
	first, _ := errsRaw[0].(map[string]any)
	if got, _ := first["error"].(string); got != rawErr {
		t.Fatalf("expected raw error %q, got %q", rawErr, got)
	}
	if got, _ := first["agent_name"].(string); got != "BMW" {
		t.Fatalf("expected agent_name BMW, got %q", got)
	}
	if got, _ := first["question_id"].(string); got != "q-1" {
		t.Fatalf("expected question_id q-1, got %q", got)
	}
}
