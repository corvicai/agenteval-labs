package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"benchmarking-platform/internal/security"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Engine struct {
	db              *gorm.DB
	pythonRunnerURL string
	workerCount     int
	taskQueue       chan *Task
	cancelledRuns   map[uuid.UUID]bool
	agentSemaphores map[uuid.UUID]chan struct{} // Per-agent concurrency control
	mu              sync.RWMutex
	wg              sync.WaitGroup
	eventCallback   func(workspaceID uuid.UUID, eventType string, correlationID string, payload any)
	httpClient      *http.Client
}

type Task struct {
	RunID            uuid.UUID
	WorkspaceID      uuid.UUID // Needed for broadcasting queued event
	AgentID          uuid.UUID
	QuestionID       string
	QuestionText     string
	ExpectedAnswer   string
	OriginalQuestion string
	AgentAnswer      string // For evaluators: the agent's response to evaluate
	AgentConfig      map[string]any
	ProviderType     string
	MaxConcurrency   int // Max parallel requests for this agent
}

type ExecutionRequest struct {
	RequestID    string         `json:"request_id"`
	ProviderType string         `json:"provider_type"`
	Config       map[string]any `json:"config"`
	Payload      map[string]any `json:"payload"`
}

type ExecutionResponse struct {
	RequestID string         `json:"request_id"`
	Success   bool           `json:"success"`
	Answer    string         `json:"answer"`
	Error     string         `json:"error"`
	Metadata  map[string]any `json:"metadata"`
}

func NewEngine(db *gorm.DB, pythonURL string, workers int) *Engine {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Engine{
		db:              db,
		pythonRunnerURL: pythonURL,
		workerCount:     workers,
		taskQueue:       make(chan *Task, 10000),
		cancelledRuns:   make(map[uuid.UUID]bool),
		agentSemaphores: make(map[uuid.UUID]chan struct{}),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Minute,
		},
	}
}

func (e *Engine) PingRunner(ctx context.Context) error {
	if e.pythonRunnerURL == "" {
		return fmt.Errorf("python runner url not configured")
	}

	url := fmt.Sprintf("%s/health", e.pythonRunnerURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	token, _ := security.GetGoogleIDToken(e.pythonRunnerURL)
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("X-Serverless-Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("runner health check failed: status %d", resp.StatusCode)
	}

	return nil
}

func (e *Engine) SetEventCallback(cb func(workspaceID uuid.UUID, eventType string, correlationID string, payload any)) {
	e.eventCallback = cb
}

func (e *Engine) Start() {
	for i := 0; i < e.workerCount; i++ {
		go e.worker(i)
	}
	log.Printf("[ENGINE] Started %d workers", e.workerCount)
}

func (e *Engine) worker(id int) {
	for task := range e.taskQueue {
		// Check if run was cancelled
		e.mu.RLock()
		cancelled := e.cancelledRuns[task.RunID]
		e.mu.RUnlock()
		if cancelled {
			log.Printf("[WORKER-%d] Skipping cancelled task for run %s", id, task.RunID)
			continue
		}

		// Get or create semaphore for this agent
		sem := e.getAgentSemaphore(task.AgentID, task.MaxConcurrency)

		// Acquire semaphore (blocks if agent at max concurrency)
		sem <- struct{}{}

		// Micro delay to avoid burst requests
		time.Sleep(100 * time.Millisecond)

		e.executeTask(task)

		// Release semaphore
		<-sem
	}
}

// getAgentSemaphore returns or creates a semaphore for rate limiting an agent
func (e *Engine) getAgentSemaphore(agentID uuid.UUID, maxConcurrency int) chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()

	if maxConcurrency <= 0 {
		maxConcurrency = 5 // Default
	}

	if sem, exists := e.agentSemaphores[agentID]; exists {
		return sem
	}

	sem := make(chan struct{}, maxConcurrency)
	e.agentSemaphores[agentID] = sem
	log.Printf("[ENGINE] Created semaphore for agent %s with max concurrency %d", agentID, maxConcurrency)
	return sem
}

func (e *Engine) QueueTask(task *Task) {
	// Emit queued event so frontend can show "Queued" state
	if e.eventCallback != nil && task.WorkspaceID != uuid.Nil {
		e.eventCallback(task.WorkspaceID, "EVT_TASK_QUEUED", task.RunID.String(), map[string]any{
			"run_id":      task.RunID.String(),
			"agent_id":    task.AgentID.String(),
			"question_id": task.QuestionID,
		})
	}
	e.taskQueue <- task
}

func (e *Engine) CancelRun(runID uuid.UUID) {
	e.mu.Lock()
	e.cancelledRuns[runID] = true
	e.mu.Unlock()

	// Update run status in DB
	if e.db != nil {
		e.db.Model(&models.Run{}).Where("id = ?", runID).Update("status", "cancelled")
	}
	log.Printf("[ENGINE] Run %s cancelled", runID)
}

func (e *Engine) executeTask(task *Task) {
	log.Printf("[ENGINE] Executing task: Run %s, Agent %s, Question %s", task.RunID, task.AgentID, task.QuestionID)

	// Get workspace ID for event broadcasting
	var workspaceID uuid.UUID
	if e.db != nil {
		var run models.Run
		if err := e.db.First(&run, "id = ?", task.RunID).Error; err == nil {
			workspaceID = run.WorkspaceID
		}
	}

	// Send task started event
	if e.eventCallback != nil {
		e.eventCallback(workspaceID, "EVT_TASK_STARTED", task.RunID.String(), map[string]any{
			"run_id":      task.RunID.String(),
			"agent_id":    task.AgentID.String(),
			"question_id": task.QuestionID,
		})
	}

	startTime := time.Now().UTC()

	providerType := task.ProviderType
	if providerType == "evaluator" {
		providerType = "openai"
	}

	payload := map[string]any{
		"question":          task.QuestionText,
		"expected_answer":   task.ExpectedAnswer,
		"original_question": task.OriginalQuestion,
		"agent_answer":      task.AgentAnswer, // For evaluators: the response to evaluate
		"answer":            task.AgentAnswer, // Alias for clarity in evaluator payloads
		"response":          task.AgentAnswer, // Extra alias for clarity in evaluator payloads
	}

	// For evaluator tasks, send both explicit question and answer fields
	if task.ProviderType == "evaluator" {
		if strings.TrimSpace(task.OriginalQuestion) != "" {
			payload["question"] = task.OriginalQuestion
		} else if strings.TrimSpace(task.QuestionText) != "" {
			payload["question"] = task.QuestionText
		}
	}

	req := ExecutionRequest{
		RequestID:    uuid.New().String(),
		ProviderType: providerType,
		Config:       task.AgentConfig,
		Payload:      payload,
	}

	body, _ := json.Marshal(req)

	// Request for Python Runner
	pythonURL := fmt.Sprintf("%s/execute", e.pythonRunnerURL)
	reqHttp, err := http.NewRequest("POST", pythonURL, bytes.NewBuffer(body))
	var resp *http.Response
	if err == nil {
		reqHttp.Header.Set("Content-Type", "application/json")

		// Service-to-Service Authentication (Cloud Run)
		token, _ := security.GetGoogleIDToken(e.pythonRunnerURL)
		if token != "" {
			reqHttp.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			// Also add X-Serverless-Authorization for redundancy/compatibility
			reqHttp.Header.Set("X-Serverless-Authorization", fmt.Sprintf("Bearer %s", token))
		}

		resp, err = e.httpClient.Do(reqHttp)
	}

	var executionResult ExecutionResponse
	if err != nil {
		executionResult = ExecutionResponse{Success: false, Error: err.Error()}
	} else {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&executionResult)
	}

	durationMs := int(time.Since(startTime).Milliseconds())

	// Store result in DB
	runResultID := uuid.New()

	if e.db != nil {
		status := "success"
		if !executionResult.Success {
			status = "error"
		}

		metadata, _ := json.Marshal(executionResult.Metadata)

		result := models.RunResult{
			ID:         runResultID,
			RunID:      task.RunID,
			AgentID:    task.AgentID,
			QuestionID: task.QuestionID,
			Status:     status,
			Answer:     executionResult.Answer,
			Error:      executionResult.Error,
			Metadata:   datatypes.JSON(metadata),
			DurationMs: durationMs,
		}

		if err := e.db.Create(&result).Error; err != nil {
			log.Printf("[ENGINE] Failed to save result: %v", err)
		}

		// Check if all tasks for this run are complete
		e.checkRunCompletion(task.RunID)
	}

	// Send task completed event
	if e.eventCallback != nil {
		e.eventCallback(workspaceID, "EVT_TASK_COMPLETED", task.RunID.String(), map[string]any{
			"run_id":        task.RunID.String(),
			"run_result_id": runResultID.String(),
			"agent_id":      task.AgentID.String(),
			"question_id":   task.QuestionID,
			"success":       executionResult.Success,
			"answer":        executionResult.Answer,
			"error":         executionResult.Error,
			"metadata":      executionResult.Metadata,
			"duration_ms":   durationMs,
		})
	}

	log.Printf("[ENGINE] Task completed: Run %s, Agent %s, Question %s, Success=%v, Duration=%dms",
		task.RunID, task.AgentID, task.QuestionID, executionResult.Success, durationMs)
}

func (e *Engine) checkRunCompletion(runID uuid.UUID) {
	if e.db == nil {
		return
	}

	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return
	}

	// Count expected vs completed results
	var count int64
	e.db.Model(&models.RunResult{}).Where("run_id = ?", runID).Count(&count)

	if int(count) >= run.TotalTasks {
		// Check for any errors
		var errorCount int64
		e.db.Model(&models.RunResult{}).Where("run_id = ? AND status = ?", runID, "error").Count(&errorCount)

		newStatus := "completed"
		if errorCount > 0 {
			newStatus = "completed_with_errors"
		}

		// Update status to completed (or with errors)
		e.db.Model(&models.Run{}).Where("id = ?", runID).Update("status", newStatus)
		log.Printf("[ENGINE] Run %s finished (%d/%d results). Status: %s", runID, count, run.TotalTasks, newStatus)

		// Emit run finished event
		if e.eventCallback != nil {
			e.eventCallback(run.WorkspaceID, "EVT_RUN_FINISHED", runID.String(), map[string]any{
				"run_id":      runID.String(),
				"total_tasks": run.TotalTasks,
				"completed":   count,
				"status":      newStatus,
			})
		}
	}
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			case json.Number:
				if s := strings.TrimSpace(t.String()); s != "" {
					return s
				}
			case float64:
				if t == float64(int64(t)) {
					return fmt.Sprintf("%.0f", t)
				}
				return fmt.Sprintf("%v", t)
			case int, int32, int64, uint, uint32, uint64, bool:
				return fmt.Sprint(t)
			default:
				if s := strings.TrimSpace(fmt.Sprint(t)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	return ""
}

func stripEvalPrefix(questionID string) string {
	if !strings.HasPrefix(questionID, "eval-") {
		return ""
	}
	rest := strings.TrimPrefix(questionID, "eval-")
	// rest should be "<uuid>-<question_id>"
	if len(rest) <= 37 {
		return ""
	}
	if rest[36] != '-' {
		return ""
	}
	return rest[37:]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractTargetAgentID(questionID string) (uuid.UUID, string) {
	if !strings.HasPrefix(questionID, "eval-") {
		return uuid.Nil, ""
	}
	rest := strings.TrimPrefix(questionID, "eval-")
	if len(rest) < 37 {
		return uuid.Nil, ""
	}
	idStr := rest[:36]
	targetID, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, ""
	}
	if rest[36] != '-' {
		return uuid.Nil, ""
	}
	return targetID, rest[37:]
}

func parseQuestionSetMaps(data datatypes.JSON) (map[string]string, map[string]string) {
	originalQuestions := make(map[string]string)
	expectedAnswers := make(map[string]string)

	if len(data) == 0 {
		return originalQuestions, expectedAnswers
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return originalQuestions, expectedAnswers
	}

	// Handle string-encoded JSON
	if s, ok := root.(string); ok {
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err == nil {
			root = decoded
		}
	}

	rootMap, ok := root.(map[string]any)
	if !ok {
		return originalQuestions, expectedAnswers
	}

	categoriesAny, _ := rootMap["categories"].([]any)
	for catIdx, catAny := range categoriesAny {
		catMap, _ := catAny.(map[string]any)
		questionsAny, _ := catMap["questions"].([]any)
		for qIdx, qAny := range questionsAny {
			fallbackID := fmt.Sprintf("%d-%d", catIdx+1, qIdx+1)
			qID := fallbackID
			questionText := ""
			expected := ""

			switch q := qAny.(type) {
			case map[string]any:
				if id := firstNonEmptyString(q, "id", "question_id", "qid"); id != "" {
					qID = id
				}
				questionText = firstNonEmptyString(q, "question", "text", "prompt")
				expected = firstNonEmptyString(q, "expected", "expected_answer", "expectedAnswer", "answer")
			case string:
				questionText = strings.TrimSpace(q)
			default:
				questionText = strings.TrimSpace(fmt.Sprint(q))
			}

			if qID == "" {
				qID = fallbackID
			}
			if qID == "" {
				continue
			}

			originalQuestions[qID] = questionText
			expectedAnswers[qID] = expected

			if qID != fallbackID {
				if _, exists := originalQuestions[fallbackID]; !exists {
					originalQuestions[fallbackID] = questionText
					expectedAnswers[fallbackID] = expected
				}
			}
		}
	}

	return originalQuestions, expectedAnswers
}

// RunEvaluators runs evaluator agents on the results of primary agents
func (e *Engine) RunEvaluators(runID uuid.UUID) error {
	if e.db == nil {
		return fmt.Errorf("database not configured")
	}

	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return err
	}

	// Get all evaluator agents for this workspace
	var evaluators []models.Agent
	if err := e.db.Where("workspace_id = ? AND provider_type = 'evaluator' AND enabled = true", run.WorkspaceID).Find(&evaluators).Error; err != nil {
		return err
	}

	// Validate evaluator configurations
	for _, eval := range evaluators {
		if err := e.validateEvaluatorConfig(eval); err != nil {
			return err
		}
	}

	// Get all results from this run
	var results []models.RunResult
	if err := e.db.Where("run_id = ?", runID).Find(&results).Error; err != nil {
		return err
	}

	// Get question set data to find expected answers
	var qs models.QuestionSet
	if err := e.db.First(&qs, "id = ?", run.QuestionSetID).Error; err != nil {
		return err
	}

	// Create a map for quick lookup of expected answers and questions
	originalQuestions, expectedAnswers := parseQuestionSetMaps(qs.Data)

	evaluatorIDs := make(map[uuid.UUID]struct{})
	for _, ev := range evaluators {
		evaluatorIDs[ev.ID] = struct{}{}
	}

	log.Printf("[EVAL] Running evaluators for run %s. Results to evaluate: %d", runID, len(results))

	// For each evaluator, create tasks to evaluate each result
	for _, evaluator := range evaluators {
		var evalConfig map[string]any

		// Check for question set override
		var override models.QuestionSetAgent
		if err := e.db.Where("question_set_id = ? AND agent_id = ?", run.QuestionSetID, evaluator.ID).First(&override).Error; err == nil {
			if !override.Enabled {
				continue // Skip if explicitly disabled for this question set
			}
			if len(override.Config) > 0 {
				json.Unmarshal(override.Config, &evalConfig)
			} else {
				json.Unmarshal(evaluator.Config, &evalConfig)
			}
		} else {
			// If no override, respect workspace-level enabled status (already filtered in query)
			json.Unmarshal(evaluator.Config, &evalConfig)
		}

		// Keep track of unique agents in this run for debugging
		runAgents := make(map[uuid.UUID]bool)
		for _, r := range results {
			runAgents[r.AgentID] = true
		}
		availableAgents := []uuid.UUID{}
		for id := range runAgents {
			availableAgents = append(availableAgents, id)
		}

		// Get target agent ID from evaluator config
		targetAgentIDStr, _ := evalConfig["target_agent_id"].(string)
		targetAgentID, _ := uuid.Parse(targetAgentIDStr)
		log.Printf("[EVAL] Evaluator %s (%s) target_agent_id: %s. Available agents in run: %v", evaluator.ID, evaluator.Name, targetAgentIDStr, availableAgents)

		if targetAgentID != uuid.Nil && !runAgents[targetAgentID] {
			log.Printf("[EVAL] WARNING: Target agent %s is NOT in this run. Skipping evaluator %s.", targetAgentID, evaluator.Name)
			continue
		}

		for _, result := range results {
			if result.Status != "success" || strings.TrimSpace(result.Answer) == "" {
				continue
			}

			if _, isEvaluator := evaluatorIDs[result.AgentID]; isEvaluator {
				continue
			}

			// Only evaluate results from the target agent
			if targetAgentID != uuid.Nil && result.AgentID != targetAgentID {
				continue
			}

			questionID := result.QuestionID
			originalQuestion, ok := originalQuestions[questionID]
			expectedAnswer := expectedAnswers[questionID]
			if !ok {
				if altID := stripEvalPrefix(questionID); altID != "" {
					if oq, okAlt := originalQuestions[altID]; okAlt {
						questionID = altID
						originalQuestion = oq
						expectedAnswer = expectedAnswers[altID]
						ok = true
					}
				}
			}

			if !ok {
				var sampleKey string
				for k := range originalQuestions {
					sampleKey = k
					break
				}
				log.Printf("[ORCHESTRATOR-DEBUG] QuestionID '%s' NOT FOUND in originalQuestions map! Map has %d entries. Sample key: '%s'", result.QuestionID, len(originalQuestions), sampleKey)
				continue
			}

			// Build evaluation prompt (now delegating more context to the worker)
			evalQuestion := result.Answer // The answer we are evaluating

			task := &Task{
				RunID:            runID,
				AgentID:          evaluator.ID,
				QuestionID:       fmt.Sprintf("eval-%s-%s", result.AgentID, questionID),
				QuestionText:     evalQuestion,
				ExpectedAnswer:   expectedAnswer,
				OriginalQuestion: originalQuestion,
				AgentAnswer:      result.Answer, // Explicit agent answer for Python server
				AgentConfig:      evalConfig,
				ProviderType:     evaluator.ProviderType,
			}
			e.QueueTask(task)
		}
	}

	return nil
}

// StartRun starts a new benchmark run
func (e *Engine) StartRun(workspaceID uuid.UUID, questionSetID uuid.UUID, agentIDs []uuid.UUID) (*models.Run, error) {
	// Get question set
	var questionSet models.QuestionSet
	if err := e.db.First(&questionSet, "id = ?", questionSetID).Error; err != nil {
		return nil, fmt.Errorf("question set not found")
	}

	// Get QuestionSetAgent overrides
	var overrides []models.QuestionSetAgent
	e.db.Where("question_set_id = ?", questionSetID).Find(&overrides)
	overrideMap := make(map[uuid.UUID]models.QuestionSetAgent)
	for _, o := range overrides {
		overrideMap[o.AgentID] = o
	}

	// Get agents
	var agents []models.Agent
	if len(agentIDs) > 0 {
		e.db.Where("id IN ? AND workspace_id = ? AND enabled = true", agentIDs, workspaceID).Find(&agents)
	} else {
		// Get all enabled primary agents (not evaluators)
		var workspaceAgents []models.Agent
		e.db.Where("workspace_id = ? AND enabled = true AND provider_type != 'evaluator'", workspaceID).Order("position ASC").Find(&workspaceAgents)

		// Filter based on overrides if any exist
		if len(overrides) > 0 {
			for _, wa := range workspaceAgents {
				if o, ok := overrideMap[wa.ID]; ok {
					if o.Enabled {
						agents = append(agents, wa)
					}
				} else {
					// No override means enabled by default (compatibility with older question sets)
					agents = append(agents, wa)
				}
			}
		} else {
			// No overrides at all - use all workspace agents
			agents = workspaceAgents
		}
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}

	// Parse questions from question set
	var qsData struct {
		Categories []struct {
			Questions []struct {
				ID       any    `json:"id"`
				Question string `json:"question"`
			} `json:"questions"`
		} `json:"categories"`
	}
	json.Unmarshal(questionSet.Data, &qsData)

	// Calculate Total Tasks
	totalTasks := 0
	nonEvaluatorAgentCount := 0
	for _, agent := range agents {
		if agent.ProviderType != "evaluator" {
			nonEvaluatorAgentCount++
		}
	}

	for _, cat := range qsData.Categories {
		totalTasks += len(cat.Questions)
	}
	totalTasks = totalTasks * nonEvaluatorAgentCount

	// Create run record
	run := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		QuestionSetID: questionSetID,
		Status:        "running",
		TotalTasks:    totalTasks,
	}
	if err := e.db.Create(&run).Error; err != nil {
		return nil, err
	}

	// Queue tasks for each agent + question
	for _, agent := range agents {
		if agent.ProviderType == "evaluator" {
			log.Printf("[ENGINE] Skipping evaluator agent %s during initial StartRun", agent.ID)
			continue
		}
		var agentConfig map[string]any

		// Use override if available
		if override, ok := overrideMap[agent.ID]; ok && len(override.Config) > 0 {
			json.Unmarshal(override.Config, &agentConfig)
		} else {
			json.Unmarshal(agent.Config, &agentConfig)
		}

		globalQuestionIndex := 0
		for catIdx, cat := range qsData.Categories {
			for qIdx, q := range cat.Questions {
				qID := ""
				switch v := q.ID.(type) {
				case string:
					if v != "" {
						qID = v
					}
				case float64:
					qID = fmt.Sprintf("%.0f", v)
				case int:
					qID = fmt.Sprintf("%d", v)
				}

				// Fallback: generate ID from category and question index
				if qID == "" {
					qID = fmt.Sprintf("%d-%d", catIdx+1, qIdx+1)
				}
				globalQuestionIndex++

				task := &Task{
					RunID:          run.ID,
					WorkspaceID:    workspaceID,
					AgentID:        agent.ID,
					QuestionID:     qID,
					QuestionText:   q.Question,
					AgentConfig:    agentConfig,
					ProviderType:   agent.ProviderType,
					MaxConcurrency: agent.MaxConcurrency,
				}
				e.QueueTask(task)
			}
		}
	}

	return &run, nil
}

// RerunTaskOptions contains optional context from the frontend
type RerunTaskOptions struct {
	OriginalQuestion string
	ExpectedAnswer   string
	QuestionSetID    string
	ResultID         string
}

// RerunTask reruns a single question for a specific agent
func (e *Engine) RerunTask(runID uuid.UUID, agentID uuid.UUID, questionID string, opts *RerunTaskOptions) error {
	// Get existing run
	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return fmt.Errorf("run not found")
	}

	// Get agent
	var agent models.Agent
	if err := e.db.First(&agent, "id = ?", agentID).Error; err != nil {
		return fmt.Errorf("agent not found")
	}

	// Optional: Use result_id to locate the exact answer to evaluate
	var resultFromPayload *models.RunResult
	if opts != nil && strings.TrimSpace(opts.ResultID) != "" {
		if rid, err := uuid.Parse(opts.ResultID); err == nil {
			var rr models.RunResult
			if err := e.db.First(&rr, "id = ?", rid).Error; err == nil {
				if rr.RunID != run.ID {
					log.Printf("[RERUN] WARNING: result_id %s does not belong to run %s (got run %s). Ignoring.", rid, run.ID, rr.RunID)
				} else {
					resultFromPayload = &rr
					if questionID == "" || questionID != rr.QuestionID {
						log.Printf("[RERUN] Overriding question_id from result_id: %s -> %s", questionID, rr.QuestionID)
						questionID = rr.QuestionID
					}
				}
			} else {
				log.Printf("[RERUN] WARNING: result_id %s not found: %v", rid, err)
			}
		} else {
			log.Printf("[RERUN] WARNING: invalid result_id %q: %v", opts.ResultID, err)
		}
	}

	// Try to use frontend-provided values first
	var questionText, expectedAnswer string
	if opts != nil && opts.OriginalQuestion != "" {
		questionText = opts.OriginalQuestion
		expectedAnswer = opts.ExpectedAnswer
		log.Printf("[RERUN] Using frontend-provided context: question=%q, expected=%q", truncate(questionText, 50), truncate(expectedAnswer, 50))
	} else {
		// Fallback: Get question set to find the question text
		var questionSet models.QuestionSet
		if err := e.db.First(&questionSet, "id = ?", run.QuestionSetID).Error; err != nil {
			return fmt.Errorf("question set not found")
		}

		// Use robust unmarshalling
		originalQuestions, expectedAnswers := parseQuestionSetMaps(questionSet.Data)

		var found bool
		questionText, found = originalQuestions[questionID]
		expectedAnswer = expectedAnswers[questionID]
		if !found {
			if altID := stripEvalPrefix(questionID); altID != "" {
				if qt, ok := originalQuestions[altID]; ok {
					questionID = altID
					questionText = qt
					expectedAnswer = expectedAnswers[altID]
					found = true
				}
			}
		}

		if !found || strings.TrimSpace(questionText) == "" {
			return fmt.Errorf("question not found")
		}
		log.Printf("[RERUN] Using DB-resolved context: question=%q, expected=%q", truncate(questionText, 50), truncate(expectedAnswer, 50))
	}

	var agentConfig map[string]any
	json.Unmarshal(agent.Config, &agentConfig)

	// Prepare Task fields
	taskQuestionText := questionText
	taskOriginalQuestion := questionText
	taskAgentAnswer := "" // For evaluators: the agent response to evaluate

	// SPECIAL HANDLING FOR EVALUATOR AGENTS
	if agent.ProviderType == "evaluator" {
		log.Printf("[RERUN] Evaluator detected: %s. Run: %s, Q: %s", agent.ID, run.ID, questionID)

		// 1. Resolve target QuestionID and AgentID
		var targetQuestionID string = questionID
		var targetAgentID uuid.UUID

		// Try to extract from the result provided by frontend if any
		if resultFromPayload != nil {
			targetQuestionID = resultFromPayload.QuestionID
			targetAgentID, _ = extractTargetAgentID(resultFromPayload.QuestionID)
			log.Printf("[RERUN] Using resultFromPayload %s (agent=%s, qid=%s)", resultFromPayload.ID, targetAgentID, targetQuestionID)
		}

		// Try to extract from the questionID provided by frontend if no target found yet
		if targetAgentID == uuid.Nil {
			if extractedAgentID, realQID := extractTargetAgentID(questionID); extractedAgentID != uuid.Nil {
				targetQuestionID = realQID
				targetAgentID = extractedAgentID
				log.Printf("[RERUN] Extracted from questionID prefix: target_agent=%s, qid=%s", targetAgentID, targetQuestionID)
			}
		}

		// Fallback to config if target agent still unknown
		if targetAgentID == uuid.Nil {
			if cid, ok := agentConfig["target_agent_id"]; ok {
				targetAgentIDStr := fmt.Sprintf("%v", cid)
				targetAgentID, _ = uuid.Parse(targetAgentIDStr)
				if targetAgentID != uuid.Nil {
					log.Printf("[RERUN] Using config target_agent_id: %s", targetAgentID)
				}
			}
		}

		// 2. Query for the target result
		if targetAgentID != uuid.Nil {
			var targetResult models.RunResult
			log.Printf("[RERUN] Looking for specific target result: run=%s, agent=%s, q=%s", run.ID, targetAgentID, targetQuestionID)

			// Try with both original ID and prefixed ID
			query := e.db.Where("run_id = ? AND agent_id = ? AND (question_id = ? OR question_id = ?)",
				run.ID, targetAgentID, targetQuestionID, "eval-"+targetAgentID.String()+"-"+targetQuestionID)

			if err := query.First(&targetResult).Error; err == nil {
				taskAgentAnswer = targetResult.Answer
				taskQuestionText = targetResult.Answer
				log.Printf("[RERUN] Found specific result: %s (Ans len: %d)", targetResult.ID, len(taskAgentAnswer))
			} else {
				log.Printf("[RERUN] Could not find specific result: %v", err)
			}
		}

		// 3. Final Heuristic: Searching for ANY non-evaluator answer for this question ID (or similar) in this run
		if taskAgentAnswer == "" {
			log.Printf("[RERUN] Starting HEURISTIC search for question %s in run %s", targetQuestionID, run.ID)
			var candidates []models.RunResult
			// Search for any results where question_id matches or ends with our targetQuestionID
			searchPattern := "%" + targetQuestionID
			if err := e.db.Where("run_id = ? AND (question_id = ? OR question_id LIKE ?) AND answer != ''", run.ID, targetQuestionID, searchPattern).Find(&candidates).Error; err == nil {
				log.Printf("[RERUN] Heuristic found %d candidates", len(candidates))
				for _, r := range candidates {
					if r.AgentID == agent.ID {
						continue // Skip self
					}
					// Verify if this agent is NOT an evaluator
					var rAgent models.Agent
					if err := e.db.First(&rAgent, "id = ?", r.AgentID).Error; err == nil {
						if rAgent.ProviderType != "evaluator" {
							taskAgentAnswer = r.Answer
							taskQuestionText = r.Answer
							log.Printf("[RERUN] HEURISTIC SUCCESS! Selected result %s from Agent %s", r.ID, r.AgentID)
							break
						}
					}
				}
			} else {
				log.Printf("[RERUN] Heuristic lookup failed: %v", err)
			}
		}

		log.Printf("[RERUN] Final Evaluator Resolution: Answer length = %d", len(taskAgentAnswer))
		if len(taskAgentAnswer) == 0 {
			log.Printf("[RERUN] !!! WARNING: NO CONTENT TO EVALUATE !!!")
		}
	}

	task := &Task{
		RunID:            run.ID,
		WorkspaceID:      run.WorkspaceID,
		AgentID:          agent.ID,
		QuestionID:       questionID,
		QuestionText:     taskQuestionText,
		OriginalQuestion: taskOriginalQuestion,
		ExpectedAnswer:   expectedAnswer,
		AgentAnswer:      taskAgentAnswer,
		AgentConfig:      agentConfig,
		ProviderType:     agent.ProviderType,
		MaxConcurrency:   agent.MaxConcurrency,
	}

	log.Printf("[RERUN] Queuing task: run=%s, agent=%s, qid=%s, ans_len=%d", task.RunID, task.AgentID, task.QuestionID, len(task.AgentAnswer))
	e.QueueTask(task)

	return nil
}

func (e *Engine) validateEvaluatorConfig(evaluator models.Agent) error {
	configStr := string(evaluator.Config)
	var config map[string]any
	if err := json.Unmarshal(evaluator.Config, &config); err != nil {
		return fmt.Errorf("invalid evaluator config for %s: %v", evaluator.Name, err)
	}

	apiKey, _ := config["api_key"].(string)

	// If "MOCK" or "DRYRUN" is explicitly present anywhere in the config (case insensitive), we allow it
	upperConfig := strings.ToUpper(configStr)
	if strings.Contains(upperConfig, "MOCK") || strings.Contains(upperConfig, "DRYRUN") {
		return nil
	}

	// Otherwise, it must be a real configuration
	if apiKey == "" {
		return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set a valid OpenAI API Key in Agent Settings or use 'MOCK' for testing.", evaluator.Name)
	}
	return nil
}
