package orchestrator

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestShouldCloseMCPSession(t *testing.T) {
	if shouldCloseMCPSession("") {
		t.Fatal("expected empty session id to skip MCP DELETE close")
	}
	if shouldCloseMCPSession("   ") {
		t.Fatal("expected blank session id to skip MCP DELETE close")
	}
	if !shouldCloseMCPSession("session-v2") {
		t.Fatal("expected non-empty session id to allow MCP DELETE close")
	}
}

func TestGoRunnerExecuteOpenRouterWithMockKey(t *testing.T) {
	runner := newGoRunner()

	resp, err := runner.Execute(context.Background(), ExecutionRequest{
		ProviderType: "openrouter",
		Config: map[string]any{
			"openrouter_api_key": "MOCK",
			"openrouter_model":   "openai/gpt-4o-mini",
		},
		Payload: map[string]any{
			"question": "What is 2 + 2?",
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got error: %s", resp.Error)
	}
	if resp.Answer == "" {
		t.Fatalf("expected non-empty answer")
	}
}

func TestGoRunnerExecuteOpenAICompatibleWithMockKey(t *testing.T) {
	runner := newGoRunner()

	resp, err := runner.Execute(context.Background(), ExecutionRequest{
		ProviderType: "openai_compatible",
		Config: map[string]any{
			"compatible_api_key":  "MOCK",
			"compatible_base_url": "https://example.ai/v1",
			"compatible_model":    "gpt-4o-mini",
		},
		Payload: map[string]any{
			"question": "What is 2 + 2?",
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got error: %s", resp.Error)
	}
	if resp.Answer == "" {
		t.Fatalf("expected non-empty answer")
	}
}

func TestGoRunnerExecuteAnthropicWithMockKey(t *testing.T) {
	runner := newGoRunner()

	resp, err := runner.Execute(context.Background(), ExecutionRequest{
		ProviderType: "anthropic",
		Config: map[string]any{
			"anthropic_api_key": "MOCK",
			"anthropic_model":   "claude-3-5-sonnet-latest",
		},
		Payload: map[string]any{
			"question": "What is 2 + 2?",
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got error: %s", resp.Error)
	}
	if resp.Answer == "" {
		t.Fatalf("expected non-empty answer")
	}
}

func TestMCPToolResultError_WhenProtocolMarksToolError(t *testing.T) {
	res := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Error calling tool 'query': agent failed to answer reason=Agent failed to compose a response"},
		},
	}

	err := mcpToolResultError(res, extractTextFromMCP(res))
	if err == nil {
		t.Fatal("expected MCP tool error to be detected")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestMCPToolResultError_WhenServerEmbedsErrorAsPlainText(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Error calling tool 'query': agent failed to answer reason=Agent failed to compose a response"},
		},
	}

	err := mcpToolResultError(res, extractTextFromMCP(res))
	if err == nil {
		t.Fatal("expected embedded MCP error text to be detected")
	}
}
