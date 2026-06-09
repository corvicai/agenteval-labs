package orchestrator

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"benchmarking-platform/internal/logger"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type goRunner struct {
	appEnv string
}

//go:embed prompts/evaluator_system_prompt.txt
var defaultEvaluatorSystemPrompt string

func DefaultEvaluatorSystemPrompt() string {
	return strings.TrimSpace(defaultEvaluatorSystemPrompt)
}

func newGoRunner() *goRunner {
	return &goRunner{appEnv: strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))}
}

func (r *goRunner) Health() error {
	return nil
}

func shouldCloseMCPSession(sessionID string) bool {
	return strings.TrimSpace(sessionID) != ""
}

func (r *goRunner) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error) {
	ctx, cancel := ensureRunnerContext(ctx)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(req.ProviderType)) {
	case "mcp":
		return r.executeMCP(ctx, req), nil
	case "openai":
		return r.executeOpenAI(ctx, req), nil
	case "nvidia":
		return r.executeNVIDIA(ctx, req), nil
	case "openrouter":
		return r.executeOpenRouter(ctx, req), nil
	case "anthropic":
		return r.executeAnthropic(ctx, req), nil
	case "openai_compatible":
		return r.executeOpenAICompatible(ctx, req), nil
	default:
		return ExecutionResponse{Success: false, Error: fmt.Sprintf("unknown provider type: %s", req.ProviderType)}, nil
	}
}

func (r *goRunner) executeMCP(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()
	endpoint := firstNonEmptyString(req.Config, "endpoint")
	token := firstNonEmptyString(req.Config, "token")
	toolName := firstNonEmptyString(req.Config, "mcp_tool_name", "tool_name")
	if toolName == "" {
		toolName = "query"
	}
	queryArg := firstNonEmptyString(req.Config, "mcp_query_arg", "query_arg")
	if queryArg == "" {
		queryArg = "query_content"
	}
	questionText := resolveQuestionText(req.Payload)

	if endpoint == "" {
		return ExecutionResponse{Success: false, Error: "MCP endpoint is missing", Metadata: durationMeta(start)}
	}

	if isMockToken(token) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockMCPAnswer()
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"content": []map[string]any{{"type": "text", "text": answer}},
					"isError": false,
				},
			},
		}
	}

	if ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	maxRetries := 3
	for retry := 0; retry <= maxRetries; retry++ {
		if ctx.Err() != nil {
			return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
		}

		result, metadata, err := r.callMCP(ctx, endpoint, token, toolName, queryArg, questionText, retry)
		if err == nil {
			result.Metadata = metadata
			result.Metadata["duration_ms"] = int(time.Since(start).Milliseconds())
			return result
		}

		if ctx.Err() != nil {
			return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
		}

		if isRateLimitError(err) && retry < maxRetries {
			waitTime := time.Duration(1<<uint(retry+1)) * time.Second
			logger.Warn("[GO RUNNER] MCP rate limited. Retry %d/%d in %s", retry+1, maxRetries, waitTime)
			select {
			case <-ctx.Done():
				return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
			case <-time.After(waitTime):
			}
			continue
		}

		errMeta := errorMeta(start, err, nil)
		addTimeoutMeta(errMeta, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			errMeta["timeout_source"] = "upstream_gateway"
		}
		errMeta["retry_count"] = retry
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: errMeta}
	}

	return ExecutionResponse{Success: false, Error: "Max retries exceeded", Metadata: durationMeta(start)}
}

func (r *goRunner) callMCP(ctx context.Context, endpoint, token, toolName, queryArg, question string, retry int) (ExecutionResponse, map[string]any, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "agenteval-go-runner", Version: "v1.0.0"}, nil)

	httpClient := &http.Client{
		Timeout: runnerTaskTimeout,
		Transport: &authTransport{
			token: token,
			base:  guardedHTTPTransport(),
		},
	}

	if ctx == nil {
		ctx = context.Background()
	}

	connCtx, cancelConn := context.WithCancel(ctx)

	session, err := client.Connect(connCtx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		cancelConn()
		return ExecutionResponse{}, nil, err
	}
	defer cancelConn()
	defer func() {
		sessionID := session.ID()
		if !shouldCloseMCPSession(sessionID) {
			logger.Debug("[GO RUNNER] MCP server at %s is stateless; skipping DELETE close", endpoint)
			return
		}
		if err := session.Close(); err != nil {
			logger.Debug("[GO RUNNER] MCP session close failed for %s (session_id=%s): %v", endpoint, strings.TrimSpace(sessionID), err)
		}
	}()

	logger.Debug("[GO RUNNER] MCP query: %s", truncate(question, 200))
	logTimeouts(ctx, httpClient.Timeout)

	callStart := time.Now()
	progressStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressStop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Debug("[GO RUNNER] Still processing MCP... %ds", int(time.Since(callStart).Seconds()))
			}
		}
	}()

	res, err := session.CallTool(connCtx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			queryArg: question,
		},
	})
	close(progressStop)

	if err != nil {
		return ExecutionResponse{}, nil, err
	}

	answerText := extractTextFromMCP(res)
	if answerText == "" {
		answerText = fmt.Sprint(res)
	}

	rawResponse := map[string]any{}
	if payload, err := json.Marshal(res); err == nil {
		if unmarshalErr := json.Unmarshal(payload, &rawResponse); unmarshalErr != nil {
			logger.Debug("[MCP] Failed to re-unmarshal response for metadata: %v", unmarshalErr)
		}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(callStart).Milliseconds()),
		"raw_response": rawResponse,
		"retry_count":  retry,
	}

	if toolErr := mcpToolResultError(res, answerText); toolErr != nil {
		metadata["mcp_tool_error"] = true
		return ExecutionResponse{
			Success:  false,
			Error:    toolErr.Error(),
			Metadata: metadata,
		}, metadata, nil
	}

	return ExecutionResponse{
		Success:  true,
		Answer:   answerText,
		Metadata: metadata,
	}, metadata, nil
}

func (r *goRunner) executeOpenAI(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()
	payload := req.Payload
	isEvaluatorTask := isEvaluatorPayload(payload)

	apiKey := firstNonEmptyString(req.Config, "openai_api_key", "api_key")
	promptID := firstNonEmptyString(req.Config, "openai_prompt_id", "prompt_id")
	promptVersion := firstNonEmptyString(req.Config, "openai_prompt_version", "prompt_version")
	projectID := firstNonEmptyString(req.Config, "openai_project_id", "project_id")
	systemPrompt := firstNonEmptyString(req.Config, "openai_system_prompt", "system_prompt", "instructions")
	if strings.TrimSpace(systemPrompt) == "" && isEvaluatorTask {
		systemPrompt = DefaultEvaluatorSystemPrompt()
	}
	model := strings.TrimSpace(firstNonEmptyString(req.Config, "openai_model", "model"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	openAIMode := resolveOpenAIMode(req.Config, promptID)

	questionText := resolveQuestionText(payload)
	originalQuestion := firstNonEmptyString(payload, "original_question")
	expectedAnswer := firstNonEmptyString(payload, "expected_answer")

	imageData, _ := payload["image_data"].(map[string]any)

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "OpenAI API Key is missing. Configure openai_api_key (or api_key) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if isMockAPIKey(apiKey) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockOpenAIAnswer(questionText)
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"mock":   true,
					"status": "simulated",
				},
			},
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	var resultText string
	var rawResponse map[string]any
	var err error
	var promptSent any

	if openAIMode == "managed" {
		if promptID == "" {
			return ExecutionResponse{
				Success: false,
				Error:   "Prompt ID is required when OpenAI mode is set to managed",
				Metadata: map[string]any{
					"duration_ms": int(time.Since(start).Milliseconds()),
				},
			}
		}

		var inputPayload any
		switch {
		case imageData != nil:
			dataURL, err := buildImageDataURL(imageData)
			if err != nil {
				return ExecutionResponse{Success: false, Error: err.Error(), Metadata: durationMeta(start)}
			}
			inputPayload = []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{"type": "input_image", "image_url": dataURL},
					},
				},
			}
		case isEvaluatorTask:
			inputPayload = fmt.Sprintf("RESPONSE TO EVALUATE:\n%s\n\n---\nCONTEXT:\nOriginal Question: %s\nExpected Answer: %s\n", questionText, fallbackString(originalQuestion, "N/A"), fallbackString(expectedAnswer, "N/A"))
		default:
			inputPayload = questionText
		}
		promptSent = map[string]any{
			"mode":           "managed",
			"prompt_id":      promptID,
			"prompt_version": promptVersion,
			"input":          inputPayload,
		}

		resultText, rawResponse, err = callOpenAIResponses(ctx, apiKey, projectID, promptID, promptVersion, inputPayload)
	} else {
		var inputPayload any
		switch {
		case imageData != nil:
			dataURL, err := buildImageDataURL(imageData)
			if err != nil {
				return ExecutionResponse{Success: false, Error: err.Error(), Metadata: durationMeta(start)}
			}
			inputPayload = []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{"type": "input_image", "image_url": dataURL},
					},
				},
			}
		case isEvaluatorTask:
			questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
			inputPayload = questionText
		default:
			inputPayload = questionText
		}

		promptSent = map[string]any{
			"mode":         "standard",
			"model":        model,
			"instructions": systemPrompt,
			"input":        inputPayload,
		}
		resultText, rawResponse, err = callOpenAIResponsesWithModel(ctx, apiKey, projectID, model, systemPrompt, inputPayload)
	}

	if err != nil {
		metadata := errorMeta(start, err, nil)
		addTimeoutMeta(metadata, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			metadata["timeout_source"] = "upstream_gateway"
		}
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent":  promptSent,
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
}

func (r *goRunner) executeNVIDIA(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()

	apiKey := firstNonEmptyString(req.Config, "nvidia_api_key", "api_key")
	model := strings.TrimSpace(firstNonEmptyString(req.Config, "nvidia_model", "model"))
	baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmptyString(req.Config, "nvidia_base_url", "base_url")), "/")
	systemPrompt := strings.TrimSpace(firstNonEmptyString(req.Config, "nvidia_system_prompt", "system_prompt", "instructions"))
	isEvaluatorTask := isEvaluatorPayload(req.Payload)
	if systemPrompt == "" && isEvaluatorTask {
		systemPrompt = DefaultEvaluatorSystemPrompt()
	}

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "NVIDIA NIM API key is missing. Configure nvidia_api_key (or api_key) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if model == "" {
		model = strings.TrimSpace(os.Getenv("NVIDIA_NIM_MODEL"))
	}
	if model == "" {
		model = "meta/llama-3.1-8b-instruct"
	}

	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("NVIDIA_NIM_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}

	if isMockAPIKey(apiKey) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockOpenAIAnswer(resolveQuestionText(req.Payload))
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"mock":   true,
					"status": "simulated",
				},
			},
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	questionText := resolveQuestionText(req.Payload)
	originalQuestion := firstNonEmptyString(req.Payload, "original_question")
	expectedAnswer := firstNonEmptyString(req.Payload, "expected_answer")
	if isEvaluatorTask {
		questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
	}

	messages := make([]map[string]any, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": questionText,
	})

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}

	resultText, rawResponse, err := callOpenAIWithBaseURL(ctx, apiKey, "", baseURL, "chat/completions", body, parseOpenAIChat)
	if err != nil {
		err = wrapProviderRequestError("nvidia", err)
		metadata := errorMeta(start, err, nil)
		addTimeoutMeta(metadata, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			metadata["timeout_source"] = "upstream_gateway"
		}
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent": map[string]any{
			"provider":      "nvidia",
			"model":         model,
			"base_url":      baseURL,
			"system_prompt": systemPrompt,
			"messages":      messages,
		},
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
}

func (r *goRunner) executeOpenRouter(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()

	apiKey := firstNonEmptyString(req.Config, "openrouter_api_key", "api_key")
	model := strings.TrimSpace(firstNonEmptyString(req.Config, "openrouter_model", "model"))
	baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmptyString(req.Config, "openrouter_base_url", "base_url")), "/")
	systemPrompt := strings.TrimSpace(firstNonEmptyString(req.Config, "openrouter_system_prompt", "system_prompt", "instructions"))
	httpReferer := strings.TrimSpace(firstNonEmptyString(req.Config, "openrouter_http_referer", "http_referer"))
	xTitle := strings.TrimSpace(firstNonEmptyString(req.Config, "openrouter_x_title", "x_title"))
	isEvaluatorTask := isEvaluatorPayload(req.Payload)
	if systemPrompt == "" && isEvaluatorTask {
		systemPrompt = DefaultEvaluatorSystemPrompt()
	}

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "OpenRouter API key is missing. Configure openrouter_api_key (or api_key) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	if httpReferer == "" {
		httpReferer = strings.TrimSpace(os.Getenv("OPENROUTER_HTTP_REFERER"))
	}
	if xTitle == "" {
		xTitle = strings.TrimSpace(os.Getenv("OPENROUTER_X_TITLE"))
	}

	if isMockAPIKey(apiKey) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockOpenAIAnswer(resolveQuestionText(req.Payload))
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"mock":   true,
					"status": "simulated",
				},
			},
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	questionText := resolveQuestionText(req.Payload)
	originalQuestion := firstNonEmptyString(req.Payload, "original_question")
	expectedAnswer := firstNonEmptyString(req.Payload, "expected_answer")
	if isEvaluatorTask {
		questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
	}

	messages := make([]map[string]any, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": questionText,
	})

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}

	extraHeaders := map[string]string{}
	if httpReferer != "" {
		extraHeaders["HTTP-Referer"] = httpReferer
	}
	if xTitle != "" {
		extraHeaders["X-Title"] = xTitle
	}

	resultText, rawResponse, err := callOpenAIWithBaseURLAndHeaders(ctx, apiKey, "", baseURL, "chat/completions", body, parseOpenAIChat, extraHeaders)
	if err != nil {
		err = wrapProviderRequestError("openrouter", err)
		metadata := errorMeta(start, err, nil)
		addTimeoutMeta(metadata, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			metadata["timeout_source"] = "upstream_gateway"
		}
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent": map[string]any{
			"provider":      "openrouter",
			"model":         model,
			"base_url":      baseURL,
			"system_prompt": systemPrompt,
			"messages":      messages,
		},
	}
	if len(extraHeaders) > 0 {
		metadata["provider_headers"] = extraHeaders
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
}

func (r *goRunner) executeOpenAICompatible(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()

	apiKey := firstNonEmptyString(req.Config, "compatible_api_key", "openai_compatible_api_key", "api_key")
	model := strings.TrimSpace(firstNonEmptyString(req.Config, "compatible_model", "openai_compatible_model", "model"))
	baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmptyString(req.Config, "compatible_base_url", "openai_compatible_base_url", "base_url")), "/")
	systemPrompt := strings.TrimSpace(firstNonEmptyString(req.Config, "compatible_system_prompt", "openai_compatible_system_prompt", "system_prompt", "instructions"))
	isEvaluatorTask := isEvaluatorPayload(req.Payload)
	if systemPrompt == "" && isEvaluatorTask {
		systemPrompt = DefaultEvaluatorSystemPrompt()
	}

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "OpenAI-compatible API key is missing. Configure compatible_api_key (or api_key) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_COMPATIBLE_BASE_URL")), "/")
	}
	if baseURL == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "OpenAI-compatible base URL is missing. Configure compatible_base_url (or base_url) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENAI_COMPATIBLE_MODEL"))
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	if isMockAPIKey(apiKey) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockOpenAIAnswer(resolveQuestionText(req.Payload))
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"mock":   true,
					"status": "simulated",
				},
			},
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	questionText := resolveQuestionText(req.Payload)
	originalQuestion := firstNonEmptyString(req.Payload, "original_question")
	expectedAnswer := firstNonEmptyString(req.Payload, "expected_answer")
	if isEvaluatorTask {
		questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
	}

	messages := make([]map[string]any, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": questionText,
	})

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}

	extraHeaders := extractStringMap(req.Config, "compatible_headers")
	resultText, rawResponse, err := callOpenAIWithBaseURLAndHeaders(ctx, apiKey, "", baseURL, "chat/completions", body, parseOpenAIChat, extraHeaders)
	if err != nil {
		err = wrapProviderRequestError("openai_compatible", err)
		metadata := errorMeta(start, err, nil)
		addTimeoutMeta(metadata, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			metadata["timeout_source"] = "upstream_gateway"
		}
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent": map[string]any{
			"provider":      "openai_compatible",
			"model":         model,
			"base_url":      baseURL,
			"system_prompt": systemPrompt,
			"messages":      messages,
		},
	}
	if len(extraHeaders) > 0 {
		metadata["provider_headers"] = extraHeaders
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
}

func (r *goRunner) executeAnthropic(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()

	apiKey := firstNonEmptyString(req.Config, "anthropic_api_key", "api_key")
	model := strings.TrimSpace(firstNonEmptyString(req.Config, "anthropic_model", "model"))
	baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmptyString(req.Config, "anthropic_base_url", "base_url")), "/")
	systemPrompt := strings.TrimSpace(firstNonEmptyString(req.Config, "anthropic_system_prompt", "system_prompt", "instructions"))
	version := strings.TrimSpace(firstNonEmptyString(req.Config, "anthropic_version"))
	isEvaluatorTask := isEvaluatorPayload(req.Payload)
	if systemPrompt == "" && isEvaluatorTask {
		systemPrompt = DefaultEvaluatorSystemPrompt()
	}

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "Anthropic API key is missing. Configure anthropic_api_key (or api_key) and retry.",
			Metadata: map[string]any{
				"duration_ms": 0,
			},
		}
	}

	if model == "" {
		model = strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	}
	if model == "" {
		model = "claude-3-5-sonnet-latest"
	}

	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	if version == "" {
		version = strings.TrimSpace(os.Getenv("ANTHROPIC_VERSION"))
	}
	if version == "" {
		version = "2023-06-01"
	}

	if isMockAPIKey(apiKey) {
		if r.isProduction() {
			return ExecutionResponse{
				Success: false,
				Error:   "Mock execution is disabled in production",
				Metadata: map[string]any{
					"duration_ms": 0,
				},
			}
		}
		answer := mockOpenAIAnswer(resolveQuestionText(req.Payload))
		return ExecutionResponse{
			Success: true,
			Answer:  answer,
			Metadata: map[string]any{
				"duration_ms": int(time.Since(start).Milliseconds()),
				"raw_response": map[string]any{
					"mock":   true,
					"status": "simulated",
				},
			},
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
	}

	questionText := resolveQuestionText(req.Payload)
	originalQuestion := firstNonEmptyString(req.Payload, "original_question")
	expectedAnswer := firstNonEmptyString(req.Payload, "expected_answer")
	if isEvaluatorTask {
		questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": questionText,
			},
		},
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}

	resultText, rawResponse, err := callAnthropicMessages(ctx, apiKey, baseURL, version, body)
	if err != nil {
		err = wrapProviderRequestError("anthropic", err)
		metadata := errorMeta(start, err, nil)
		addTimeoutMeta(metadata, ctx, runnerTaskTimeout)
		if isGatewayTimeoutError(err) {
			metadata["timeout_source"] = "upstream_gateway"
		}
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent": map[string]any{
			"provider":      "anthropic",
			"model":         model,
			"base_url":      baseURL,
			"anthropic_ver": version,
			"system_prompt": systemPrompt,
		},
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
}

func resolveOpenAIMode(config map[string]any, promptID string) string {
	mode := strings.ToLower(strings.TrimSpace(firstNonEmptyString(config, "openai_mode")))
	switch mode {
	case "managed", "managed_prompt", "prompt":
		return "managed"
	case "standard", "direct", "default":
		return "standard"
	}

	if strings.TrimSpace(promptID) != "" {
		return "managed"
	}
	return "standard"
}

func (r *goRunner) isProduction() bool {
	return r.appEnv == "production"
}

type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	if t.token != "" {
		req.Header.Set("Authorization", t.token)
	}
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if readErr == nil {
			bodyText := string(body)
			if req.Method == http.MethodDelete &&
				resp.StatusCode == http.StatusMethodNotAllowed &&
				strings.Contains(strings.ToLower(bodyText), "session termination not supported") {
				logger.Debug("[GO RUNNER] MCP session close not supported by %s %s: %s",
					req.Method, req.URL.String(), bodyText)
				return resp, err
			}
			if len(body) > 0 {
				logger.Warn("[GO RUNNER] MCP HTTP %d from %s %s: %s",
					resp.StatusCode, req.Method, req.URL.String(), bodyText)
			} else {
				logger.Warn("[GO RUNNER] MCP HTTP %d from %s %s (empty body)",
					resp.StatusCode, req.Method, req.URL.String())
			}
		}
	}
	return resp, err
}

func resolveQuestionText(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if answer := firstNonEmptyString(payload, "agent_answer", "answer", "response"); answer != "" {
		return strings.TrimSpace(answer)
	}
	return strings.TrimSpace(firstNonEmptyString(payload, "question"))
}

func isEvaluatorPayload(payload map[string]any) bool {
	if payload == nil {
		return false
	}

	switch raw := payload["is_evaluator_task"].(type) {
	case bool:
		return raw
	case string:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "y", "on":
			return true
		}
	}

	return strings.TrimSpace(firstNonEmptyString(payload, "agent_answer")) != ""
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit")
}

func isGatewayTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "gateway timeout") || strings.Contains(lower, "504")
}

func isMockToken(token string) bool {
	switch strings.ToUpper(strings.TrimSpace(token)) {
	case "DRYRUN", "MOCK", "MOCK_TOKEN":
		return true
	default:
		return false
	}
}

func isMockAPIKey(apiKey string) bool {
	upper := strings.ToUpper(apiKey)
	return strings.Contains(upper, "MOCK") || strings.Contains(upper, "DRYRUN")
}

func mockMCPAnswer() string {
	answers := []string{
		"The answer is correct according to the logic provided.",
		"Tokyo is indeed the capital city of Japan.",
		"The pills will last exactly one hour (0m, 30m, 60m).",
		"Leonardo da Vinci completed the Mona Lisa.",
		"299,792,458 meters per second in a vacuum.",
		"Compound interest generates exponential growth.",
		"Kubernetes manages containers across clusters.",
		"Yes, following the transitive property (A -> B -> C).",
		"George Orwell wrote 1984 in 1949.",
		"Au comes from the Latin word Aurum.",
		"TCP ensures delivery, UDP sends packets without verification.",
		"Binary search cuts the search space in half each step.",
		"Deep Learning is a subset of Machine Learning using neural networks.",
		"The Trolley Problem highlights utilitarian vs deontological ethics.",
		"Red leaves falling down / Gold and crunch under my feet / Winter is waking.",
		"The Red Bean Roastery.",
		"It smells like wet asphalt and fresh soil.",
		"To measure 4L: Fill 5, pour to 3. 2 left in 5. Empty 3. Pour 2 to 3. Fill 5. Pour 1 to 3. 4 left.",
		"O(n^2) is the worst case for bubble sort.",
		"HTTP is stateless, HTTPS is secure.",
		"AI is the simulation of human intelligence in machines.",
		"Machine learning allows computers to learn from data without explicit programming.",
		"A neural network mimics the brain's structure with layers of connected nodes.",
		"Backpropagation adjusts weights by propagating errors backward through the network.",
		"Docker containerizes applications for consistent deployment across environments.",
		"AI bias occurs when training data reflects societal prejudices.",
		"Alignment ensures AI systems act according to human values and intentions.",
	}
	return answers[rand.Intn(len(answers))] //nolint:gosec // G404: synthetic test-answer picker; cryptographic randomness not needed
}

func mockOpenAIAnswer(question string) string {
	lower := strings.ToLower(question)
	if strings.Contains(lower, "answer") || strings.Contains(lower, "expected") {
		return "PASS (MOCK): The target agent's response correctly matches the essence of the expected answer."
	}
	if strings.Contains(lower, "evalu") {
		return "Rating (MOCK): 5/5. Reason: The response is helpful, clear, and concise."
	}
	return "Evaluation complete (MOCK): The agent response appears accurate and follow instructions."
}

func extractTextFromMCP(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range res.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			b.WriteString(c.Text)
		default:
			b.WriteString(fmt.Sprint(c))
		}
	}
	return b.String()
}

func mcpToolResultError(res *mcp.CallToolResult, answerText string) error {
	if res == nil {
		return errors.New("empty MCP tool response")
	}

	trimmed := strings.TrimSpace(answerText)
	if res.IsError {
		if trimmed != "" {
			return errors.New(trimmed)
		}
		return errors.New("MCP tool returned an error")
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(trimmed, "Error calling tool '") ||
		strings.HasPrefix(trimmed, `Error calling tool "`) ||
		(strings.Contains(lower, "agent failed to answer") && strings.Contains(lower, "reason=")) {
		return errors.New(trimmed)
	}

	return nil
}

func buildImageDataURL(imageData map[string]any) (string, error) {
	if imageData == nil {
		return "", errors.New("image_data is missing")
	}
	contentType := firstNonEmptyString(imageData, "content_type", "mime_type", "mimeType")
	base64Data := firstNonEmptyString(imageData, "base64_data", "base64Data")
	if base64Data == "" {
		return "", errors.New("image_data.base64_data is required when image_data is provided")
	}
	if contentType == "" {
		contentType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64Data), nil
}

func buildEvaluationPrompt(question, originalQuestion, expectedAnswer string) string {
	if originalQuestion == "" && expectedAnswer == "" {
		return question
	}
	var b strings.Builder
	b.WriteString("EVALUATION TASK\n\n")
	if originalQuestion != "" {
		b.WriteString("**Original Question:**\n")
		b.WriteString(originalQuestion)
		b.WriteString("\n\n")
	}
	if expectedAnswer != "" {
		b.WriteString("**Expected Answer (Gabarito):**\n")
		b.WriteString(expectedAnswer)
		b.WriteString("\n\n")
	}
	b.WriteString("**Response to Evaluate:**\n")
	b.WriteString(question)
	b.WriteString("\n\n")
	b.WriteString("Please evaluate if the response correctly addresses the original question and matches the expected answer if provided. If the question is about one topic (e.g. apples) but the response is about another (e.g. bananas), mark it as a failure.")
	return b.String()
}

func wrapProviderRequestError(provider string, err error) error {
	if err == nil {
		return nil
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if message == "" || strings.HasPrefix(strings.ToLower(message), provider+" request failed") {
		return err
	}
	return fmt.Errorf("%s request failed: %w", provider, err)
}

func callOpenAIResponses(ctx context.Context, apiKey, projectID, promptID, promptVersion string, inputPayload any) (string, map[string]any, error) {
	prompt := map[string]any{
		"id": promptID,
	}
	if strings.TrimSpace(promptVersion) != "" {
		prompt["version"] = promptVersion
	}

	body := map[string]any{
		"prompt":    prompt,
		"input":     inputPayload,
		"reasoning": map[string]any{},
		"store":     true,
		"include":   []string{"web_search_call.action.sources"},
	}

	return callOpenAI(ctx, apiKey, projectID, "responses", body, parseOpenAIResponses)
}

func callOpenAIResponsesWithModel(ctx context.Context, apiKey, projectID, model, instructions string, inputPayload any) (string, map[string]any, error) {
	body := map[string]any{
		"model":     model,
		"input":     inputPayload,
		"reasoning": map[string]any{},
		"store":     true,
		"include":   []string{"web_search_call.action.sources"},
	}

	if strings.TrimSpace(instructions) != "" {
		body["instructions"] = instructions
	}

	return callOpenAI(ctx, apiKey, projectID, "responses", body, parseOpenAIResponses)
}

type parseFn func(map[string]any) (string, error)

func callOpenAI(ctx context.Context, apiKey, projectID, path string, body map[string]any, parser parseFn) (string, map[string]any, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return callOpenAIWithBaseURL(ctx, apiKey, projectID, baseURL, path, body, parser)
}

func callOpenAIWithBaseURL(ctx context.Context, apiKey, projectID, baseURL, path string, body map[string]any, parser parseFn) (string, map[string]any, error) {
	return callOpenAIWithBaseURLAndHeaders(ctx, apiKey, projectID, baseURL, path, body, parser, nil)
}

func callOpenAIWithBaseURLAndHeaders(ctx context.Context, apiKey, projectID, baseURL, path string, body map[string]any, parser parseFn, extraHeaders map[string]string) (string, map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/"+path, bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if projectID != "" {
		req.Header.Set("OpenAI-Project", projectID)
	}
	for key, value := range extraHeaders {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: runnerTaskTimeout, Transport: guardedHTTPTransport()}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", nil, fmt.Errorf("openai request failed with status %d: could not read response body: %v", resp.StatusCode, readErr)
	}

	var decoded map[string]any
	jsonErr := json.Unmarshal(bodyBytes, &decoded)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if jsonErr == nil {
			if errMsg := parseOpenAIError(decoded); errMsg != "" {
				return "", decoded, errors.New(errMsg)
			}
		}
		// Non-JSON or unparseable error body (e.g. a proxy / gateway HTML page,
		// the typical shape when Cloud Run egress is misconfigured): preserve the
		// raw body so the real cause is visible instead of a bare status code.
		return "", decoded, fmt.Errorf("openai request failed with status %d: %s", resp.StatusCode, truncate(strings.TrimSpace(string(bodyBytes)), 1500))
	}

	if jsonErr != nil {
		return "", nil, fmt.Errorf("openai returned status %d with a non-JSON body: %s", resp.StatusCode, truncate(strings.TrimSpace(string(bodyBytes)), 1500))
	}

	text, err := parser(decoded)
	return text, decoded, err
}

func callAnthropicMessages(ctx context.Context, apiKey, baseURL, version string, body map[string]any) (string, map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	anthropicPath := baseURL
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(baseURL)), "/messages") {
		anthropicPath = strings.TrimRight(baseURL, "/") + "/messages"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicPath, bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", version)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: runnerTaskTimeout, Transport: guardedHTTPTransport()}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if errMsg := parseAnthropicError(decoded); errMsg != "" {
			return "", decoded, errors.New(errMsg)
		}
		return "", decoded, fmt.Errorf("anthropic request failed with status %d", resp.StatusCode)
	}

	text, err := parseAnthropicMessages(decoded)
	return text, decoded, err
}

func parseAnthropicError(decoded map[string]any) string {
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		return ""
	}
	if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}
	return ""
}

func parseAnthropicMessages(decoded map[string]any) (string, error) {
	content, ok := decoded["content"].([]any)
	if !ok || len(content) == 0 {
		return "", errors.New("anthropic response missing content")
	}

	var b strings.Builder
	for _, item := range content {
		segment, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := segment["type"].(string)
		if strings.TrimSpace(strings.ToLower(t)) != "text" {
			continue
		}
		text, _ := segment["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(text)
	}

	if b.Len() == 0 {
		return "", errors.New("unable to extract anthropic text response")
	}
	return b.String(), nil
}

func extractStringMap(config map[string]any, key string) map[string]string {
	if config == nil {
		return nil
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}

	result := map[string]string{}
	switch typed := raw.(type) {
	case map[string]string:
		for k, v := range typed {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			result[k] = v
		}
	case map[string]any:
		for k, v := range typed {
			if strings.TrimSpace(k) == "" {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(v))
			if value == "" || value == "<nil>" {
				continue
			}
			result[k] = value
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func parseOpenAIResponses(decoded map[string]any) (string, error) {
	if outputText, ok := decoded["output_text"].(string); ok && outputText != "" {
		return outputText, nil
	}

	if output, ok := decoded["output"].([]any); ok && len(output) > 0 {
		if first, ok := output[0].(map[string]any); ok {
			if content, ok := first["content"].([]any); ok && len(content) > 0 {
				if firstContent, ok := content[0].(map[string]any); ok {
					if text, ok := firstContent["text"].(string); ok && text != "" {
						return text, nil
					}
				}
			}
		}
	}
	return "", errors.New("unable to extract response text")
}

func parseOpenAIChat(decoded map[string]any) (string, error) {
	choices, ok := decoded["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", errors.New("openai response missing choices")
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return "", errors.New("openai response malformed choices")
	}
	message, ok := first["message"].(map[string]any)
	if !ok {
		return "", errors.New("openai response missing message")
	}
	content, _ := message["content"].(string)
	if content == "" {
		return "", errors.New("openai response missing content")
	}
	return content, nil
}

func parseOpenAIError(decoded map[string]any) string {
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		return ""
	}
	if msg, ok := errObj["message"].(string); ok && msg != "" {
		return msg
	}
	return ""
}

func errorMeta(start time.Time, err error, subErrors []string) map[string]any {
	meta := map[string]any{
		"duration_ms": int(time.Since(start).Milliseconds()),
		"error_type":  fmt.Sprintf("%T", err),
		"error_detail": func() string {
			if err == nil {
				return ""
			}
			return err.Error()
		}(),
	}
	if len(subErrors) > 0 {
		meta["sub_errors"] = subErrors
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) != "production" {
		meta["traceback"] = string(debug.Stack())
	}
	return meta
}

func durationMeta(start time.Time) map[string]any {
	return map[string]any{"duration_ms": int(time.Since(start).Milliseconds())}
}

func addTimeoutMeta(meta map[string]any, ctx context.Context, httpTimeout time.Duration) {
	if meta == nil {
		return
	}
	meta["runner_task_timeout_ms"] = int(runnerTaskTimeout.Milliseconds())
	meta["http_timeout_ms"] = int(httpTimeout.Milliseconds())
	if ctx == nil {
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		meta["ctx_deadline"] = deadline.UTC().Format(time.RFC3339Nano)
		meta["ctx_remaining_ms"] = int(time.Until(deadline).Milliseconds())
	}
}

func logTimeouts(ctx context.Context, httpTimeout time.Duration) {
	if ctx == nil {
		logger.Debug("[GO RUNNER] MCP timeouts: ctx_deadline=<none> http_timeout=%s", httpTimeout)
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline).Truncate(time.Second)
		logger.Debug("[GO RUNNER] MCP timeouts: ctx_deadline=%s remaining=%s http_timeout=%s", deadline.UTC().Format(time.RFC3339), remaining, httpTimeout)
		return
	}
	logger.Debug("[GO RUNNER] MCP timeouts: ctx_deadline=<none> http_timeout=%s", httpTimeout)
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
