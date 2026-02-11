package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type goRunner struct {
	appEnv string
}

func newGoRunner() *goRunner {
	return &goRunner{appEnv: strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))}
}

func (r *goRunner) Health() error {
	return nil
}

func (r *goRunner) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error) {
	ctx, cancel := ensureRunnerContext(ctx)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(req.ProviderType)) {
	case "mcp":
		return r.executeMCP(ctx, req), nil
	case "openai":
		return r.executeOpenAI(ctx, req), nil
	default:
		return ExecutionResponse{Success: false, Error: fmt.Sprintf("unknown provider type: %s", req.ProviderType)}, nil
	}
}

func (r *goRunner) executeMCP(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()
	endpoint := firstNonEmptyString(req.Config, "endpoint")
	token := firstNonEmptyString(req.Config, "token")
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

		result, metadata, err := r.callMCP(ctx, endpoint, token, questionText, retry)
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
			log.Printf("[GO RUNNER] MCP rate limited. Retry %d/%d in %s", retry+1, maxRetries, waitTime)
			select {
			case <-ctx.Done():
				return ExecutionResponse{Success: false, Error: ctx.Err().Error(), Metadata: errorMeta(start, ctx.Err(), nil)}
			case <-time.After(waitTime):
			}
			continue
		}

		errMeta := errorMeta(start, err, nil)
		errMeta["retry_count"] = retry
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: errMeta}
	}

	return ExecutionResponse{Success: false, Error: "Max retries exceeded", Metadata: durationMeta(start)}
}

func (r *goRunner) callMCP(ctx context.Context, endpoint, token, question string, retry int) (ExecutionResponse, map[string]any, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "agenteval-runner", Version: "v1.0.0"}, nil)

	httpClient := &http.Client{
		Timeout: runnerTaskTimeout,
		Transport: &authTransport{
			token: token,
			base:  http.DefaultTransport,
		},
	}

	if ctx == nil {
		ctx = context.Background()
	}

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return ExecutionResponse{}, nil, err
	}
	defer session.Close()

	log.Printf("[GO RUNNER] MCP query: %s", truncate(question, 200))

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
				log.Printf("[GO RUNNER] Still processing MCP... %ds", int(time.Since(callStart).Seconds()))
			}
		}
	}()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "query",
		Arguments: map[string]any{
			"query_content": question,
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
		_ = json.Unmarshal(payload, &rawResponse)
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(callStart).Milliseconds()),
		"raw_response": rawResponse,
		"retry_count":  retry,
	}

	return ExecutionResponse{
		Success:  true,
		Answer:   answerText,
		Metadata: metadata,
	}, metadata, nil
}

func (r *goRunner) executeOpenAI(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	start := time.Now()
	apiKey := firstNonEmptyString(req.Config, "api_key")
	promptID := firstNonEmptyString(req.Config, "prompt_id")
	promptVersion := firstNonEmptyString(req.Config, "prompt_version")
	projectID := firstNonEmptyString(req.Config, "project_id")

	payload := req.Payload
	questionText := resolveQuestionText(payload)
	originalQuestion := firstNonEmptyString(payload, "original_question")
	expectedAnswer := firstNonEmptyString(payload, "expected_answer")

	imageData, _ := payload["image_data"].(map[string]any)

	if apiKey == "" {
		return ExecutionResponse{
			Success: false,
			Error:   "OpenAI API Key is missing. Please configure your Evaluator Agent or use 'MOCK' as the key.",
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

	if promptID != "" && promptVersion != "" {
		var inputPayload any
		if imageData != nil {
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
		} else if originalQuestion != "" || expectedAnswer != "" {
			inputPayload = fmt.Sprintf("RESPONSE TO EVALUATE:\n%s\n\n---\nCONTEXT:\nOriginal Question: %s\nExpected Answer: %s\n", questionText, fallbackString(originalQuestion, "N/A"), fallbackString(expectedAnswer, "N/A"))
		} else {
			inputPayload = questionText
		}
		promptSent = inputPayload

		resultText, rawResponse, err = callOpenAIResponses(ctx, apiKey, projectID, promptID, promptVersion, inputPayload)
	} else if imageData != nil {
		dataURL, err := buildImageDataURL(imageData)
		if err != nil {
			return ExecutionResponse{Success: false, Error: err.Error(), Metadata: durationMeta(start)}
		}
		content := []map[string]any{
			{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
		}
		promptSent = content
		resultText, rawResponse, err = callOpenAIChat(ctx, apiKey, projectID, []map[string]any{{"role": "user", "content": content}})
	} else {
		questionText = buildEvaluationPrompt(questionText, originalQuestion, expectedAnswer)
		promptSent = questionText
		resultText, rawResponse, err = callOpenAIChat(ctx, apiKey, projectID, []map[string]any{{"role": "user", "content": questionText}})
	}

	if err != nil {
		metadata := errorMeta(start, err, nil)
		return ExecutionResponse{Success: false, Error: err.Error(), Metadata: metadata}
	}

	metadata := map[string]any{
		"duration_ms":  int(time.Since(start).Milliseconds()),
		"raw_response": rawResponse,
		"prompt_sent":  promptSent,
	}
	return ExecutionResponse{Success: true, Answer: resultText, Metadata: metadata}
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
	return t.base.RoundTrip(req)
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

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit")
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
	return answers[rand.Intn(len(answers))]
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

func callOpenAIResponses(ctx context.Context, apiKey, projectID, promptID, promptVersion string, inputPayload any) (string, map[string]any, error) {
	body := map[string]any{
		"prompt": map[string]any{
			"id":      promptID,
			"version": promptVersion,
		},
		"input":     inputPayload,
		"reasoning": map[string]any{},
		"store":     true,
		"include":   []string{"web_search_call.action.sources"},
	}

	return callOpenAI(ctx, apiKey, projectID, "responses", body, parseOpenAIResponses)
}

func callOpenAIChat(ctx context.Context, apiKey, projectID string, messages []map[string]any) (string, map[string]any, error) {
	body := map[string]any{
		"model":    "gpt-4o-mini",
		"messages": messages,
	}
	return callOpenAI(ctx, apiKey, projectID, "chat/completions", body, parseOpenAIChat)
}

type parseFn func(map[string]any) (string, error)

func callOpenAI(ctx context.Context, apiKey, projectID, path string, body map[string]any, parser parseFn) (string, map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
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

	client := &http.Client{Timeout: runnerTaskTimeout}
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
		if errMsg := parseOpenAIError(decoded); errMsg != "" {
			return "", decoded, errors.New(errMsg)
		}
		return "", decoded, fmt.Errorf("openai request failed with status %d", resp.StatusCode)
	}

	text, err := parser(decoded)
	return text, decoded, err
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

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
