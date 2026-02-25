package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"
)

// DebugMCPTestPayload is the request payload for REQ_ADMIN_DEBUG_MCP_TEST.
type DebugMCPTestPayload struct {
	Endpoint string   `json:"endpoint"`
	Token    string   `json:"token"`
	Question string   `json:"question"`
	Tests    []string `json:"tests"` // subset of: "go_sdk", "raw_2025_06_18", "raw_2024_11_05"
}

// DebugMCPTestResult holds the outcome of a single probe.
type DebugMCPTestResult struct {
	Name         string         `json:"name"`
	Success      bool           `json:"success"`
	DurationMs   int            `json:"duration_ms"`
	StatusCode   int            `json:"status_code,omitempty"`
	Error        string         `json:"error,omitempty"`
	Answer       string         `json:"answer,omitempty"`
	RequestBody  string         `json:"request_body,omitempty"`
	ResponseBody string         `json:"response_body,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
}

// DebugMCPTestResponse is the response payload for DATA_ADMIN_DEBUG_MCP_RESULT.
type DebugMCPTestResponse struct {
	Endpoint string               `json:"endpoint"`
	Results  []DebugMCPTestResult `json:"results"`
}

// handleAdminDebugMCPTest runs a battery of MCP connectivity probes and returns
// detailed diagnostics. Restricted to super admins.
func (h *Hub) handleAdminDebugMCPTest(c *Connection, env models.Envelope) {
	if err := h.checkSuperAdmin(c, env); err != nil {
		return
	}

	var payload DebugMCPTestPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}
	if payload.Endpoint == "" {
		c.SendError(env.CorrelationID, "endpoint is required")
		return
	}

	tests := payload.Tests
	if len(tests) == 0 {
		tests = []string{"go_sdk", "raw_2025_06_18", "raw_2024_11_05"}
	}

	question := payload.Question
	if question == "" {
		question = "Hello, what tools do you have available?"
	}

	logger.Info("[DEBUG] Super admin %s running MCP diagnostic on %s (tests: %v)", c.UserID, payload.Endpoint, tests)

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]DebugMCPTestResult, 0, len(tests))

	for _, testType := range tests {
		wg.Add(1)
		go func(tt string) {
			defer wg.Done()
			var result DebugMCPTestResult
			switch tt {
			case "go_sdk":
				result = runMCPGoSDKTest(payload.Endpoint, payload.Token, question)
			case "raw_2025_06_18":
				result = runMCPRawHTTPTest(payload.Endpoint, payload.Token, "2025-06-18")
			case "raw_2024_11_05":
				result = runMCPRawHTTPTest(payload.Endpoint, payload.Token, "2024-11-05")
			default:
				result = DebugMCPTestResult{Name: tt, Error: "unknown test type"}
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(testType)
	}
	wg.Wait()

	// Sort results in the requested order for consistent display.
	ordered := make([]DebugMCPTestResult, 0, len(tests))
	for _, tt := range tests {
		for _, r := range results {
			if r.Name == testName(tt) {
				ordered = append(ordered, r)
				break
			}
		}
	}

	c.SendResponse(DataAdminDebugMCPResult, env.CorrelationID, DebugMCPTestResponse{
		Endpoint: payload.Endpoint,
		Results:  ordered,
	})
}

func testName(testType string) string {
	switch testType {
	case "go_sdk":
		return "Go SDK (production path)"
	case "raw_2025_06_18":
		return "Raw HTTP – MCP 2025-06-18"
	case "raw_2024_11_05":
		return "Raw HTTP – MCP 2024-11-05"
	default:
		return testType
	}
}

// runMCPGoSDKTest exercises the exact same code path used in production.
// NOTE: This makes a full round-trip including the tool call (AI inference),
// which can take minutes. We use the same timeout as the production runner.
func runMCPGoSDKTest(endpoint, token, question string) DebugMCPTestResult {
	result := DebugMCPTestResult{Name: "Go SDK (production path)"}
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), orchestrator.RunnerTaskTimeout)
	defer cancel()

	runner := orchestrator.NewGoRunner()
	resp, err := runner.Execute(ctx, orchestrator.ExecutionRequest{
		ProviderType: "mcp",
		Config: map[string]any{
			"endpoint": endpoint,
			"token":    token,
		},
		Payload: map[string]any{
			"question": question,
		},
	})

	result.DurationMs = int(time.Since(start).Milliseconds())

	if err != nil {
		result.Error = fmt.Sprintf("Execute error: %v", err)
		return result
	}

	result.Success = resp.Success
	result.Answer = resp.Answer
	if !resp.Success {
		result.Error = resp.Error
	}
	result.Details = resp.Metadata
	return result
}

// runMCPRawHTTPTest sends a raw MCP initialize JSON-RPC POST, bypassing the
// Go MCP SDK entirely. This isolates whether a failure is in the SDK or in the
// network/server layer.
func runMCPRawHTTPTest(endpoint, token, protocolVersion string) DebugMCPTestResult {
	result := DebugMCPTestResult{Name: fmt.Sprintf("Raw HTTP – MCP %s", protocolVersion)}
	start := time.Now()

	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"roots":    map[string]any{"listChanged": true},
				"sampling": map[string]any{},
			},
			"clientInfo": map[string]any{
				"name":    "agenteval-debug-probe",
				"version": "v1.0.0",
			},
		},
	}

	bodyBytes, err := json.MarshalIndent(initBody, "", "  ")
	if err != nil {
		result.Error = fmt.Sprintf("marshal error: %v", err)
		return result
	}
	result.RequestBody = string(bodyBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Error = fmt.Sprintf("build request error: %v", err)
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	result.DurationMs = int(time.Since(start).Milliseconds())
	if err != nil {
		result.Error = fmt.Sprintf("HTTP error: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	bodyData, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	result.ResponseBody = string(bodyData)
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.Success {
		result.Error = fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return result
}
