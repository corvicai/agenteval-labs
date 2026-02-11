package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"benchmarking-platform/internal/security"
)

type httpRunner struct {
	baseURL    string
	httpClient *http.Client
}

func newHTTPRunner(baseURL string) *httpRunner {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}
	return &httpRunner{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Minute,
		},
	}
}

func (r *httpRunner) Health() error {
	if r.baseURL == "" {
		return fmt.Errorf("python runner url not configured")
	}

	url := fmt.Sprintf("%s/health", r.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	token, _ := security.GetGoogleIDToken(r.baseURL)
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("X-Serverless-Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("runner health check failed: status %d", resp.StatusCode)
	}
	return nil
}

func (r *httpRunner) Execute(req ExecutionRequest) (ExecutionResponse, error) {
	if r.baseURL == "" {
		return ExecutionResponse{Success: false, Error: "python runner url not configured"}, nil
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ExecutionResponse{Success: false, Error: err.Error()}, nil
	}

	url := fmt.Sprintf("%s/execute", r.baseURL)
	reqHttp, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return ExecutionResponse{Success: false, Error: err.Error()}, nil
	}
	reqHttp.Header.Set("Content-Type", "application/json")

	token, _ := security.GetGoogleIDToken(r.baseURL)
	if token != "" {
		reqHttp.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		reqHttp.Header.Set("X-Serverless-Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := r.httpClient.Do(reqHttp)
	if err != nil {
		return ExecutionResponse{Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	var executionResult ExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&executionResult); err != nil {
		return ExecutionResponse{Success: false, Error: err.Error()}, nil
	}
	return executionResult, nil
}
