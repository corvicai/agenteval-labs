package orchestrator

import (
	"context"
	"testing"
)

func TestGoRunnerExecuteNVIDIAWithMockKey(t *testing.T) {
	runner := newGoRunner()

	resp, err := runner.Execute(context.Background(), ExecutionRequest{
		ProviderType: "nvidia",
		Config: map[string]any{
			"api_key": "MOCK",
			"model":   "meta/llama-3.1-8b-instruct",
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

func TestGoRunnerExecuteNVIDIAMissingAPIKey(t *testing.T) {
	runner := newGoRunner()

	resp, err := runner.Execute(context.Background(), ExecutionRequest{
		ProviderType: "nvidia",
		Config: map[string]any{
			"model": "meta/llama-3.1-8b-instruct",
		},
		Payload: map[string]any{
			"question": "What is 2 + 2?",
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resp.Success {
		t.Fatalf("expected failure for missing api key")
	}
}
