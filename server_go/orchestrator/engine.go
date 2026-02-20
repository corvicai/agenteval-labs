package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Engine struct {
	db              *gorm.DB
	runner          Runner
	workerCount     int
	taskQueue       chan *Task
	cancelledRuns   map[uuid.UUID]bool
	runContexts     map[uuid.UUID]context.Context
	runCancels      map[uuid.UUID]context.CancelFunc
	agentSemaphores map[uuid.UUID]chan struct{} // Per-agent concurrency control
	retryStates     map[string]retryState
	mu              sync.RWMutex
	wg              sync.WaitGroup
	eventCallback   func(workspaceID uuid.UUID, eventType string, correlationID string, payload any)
}

type Task struct {
	RunID             uuid.UUID
	WorkspaceID       uuid.UUID // Needed for broadcasting queued event
	AgentID           uuid.UUID
	QuestionID        string
	QuestionText      string
	ExpectedAnswer    string
	OriginalQuestion  string
	AgentAnswer       string // For evaluators: the agent's response to evaluate
	AgentConfig       map[string]any
	ProviderType      string
	MaxConcurrency    int // Max parallel requests for this agent
	RetryID           string
	TargetRunResultID uuid.UUID // For evaluator tasks: run_result being evaluated
}

var (
	evaluatorScoreAtEndRegex = regexp.MustCompile(`(\d{1,2})\s*/\s*10$`)
	evaluatorScoreAnyRegex   = regexp.MustCompile(`(^|[^0-9])(\d{1,2})\s*/\s*10($|[^0-9])`)
)

func decodeConfigJSON(raw []byte) map[string]any {
	cfg := make(map[string]any)
	if len(raw) == 0 {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		return map[string]any{}
	}
	return cfg
}

func mergeConfig(base map[string]any, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
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

func NewEngine(db *gorm.DB, workers int) *Engine {
	return &Engine{
		db:              db,
		runner:          newRunner(),
		workerCount:     workers,
		taskQueue:       make(chan *Task, 10000),
		cancelledRuns:   make(map[uuid.UUID]bool),
		runContexts:     make(map[uuid.UUID]context.Context),
		runCancels:      make(map[uuid.UUID]context.CancelFunc),
		agentSemaphores: make(map[uuid.UUID]chan struct{}),
		retryStates:     make(map[string]retryState),
	}
}

func (e *Engine) PingRunner() error {
	if e.runner == nil {
		return fmt.Errorf("runner not configured")
	}
	return e.runner.Health()
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
	if task != nil && task.RetryID != "" {
		e.setRetryState(
			task.RetryID,
			task.RunID,
			task.WorkspaceID,
			task.AgentID,
			task.QuestionID,
			"queued",
			uuid.Nil,
			"",
			0,
		)
	}

	// Emit queued event so frontend can show "Queued" state
	if e.eventCallback != nil && task.WorkspaceID != uuid.Nil {
		payload := map[string]any{
			"run_id":      task.RunID.String(),
			"agent_id":    task.AgentID.String(),
			"question_id": task.QuestionID,
		}
		if task.RetryID != "" {
			payload["retry_id"] = task.RetryID
		}
		e.eventCallback(task.WorkspaceID, "EVT_TASK_QUEUED", task.RunID.String(), payload)
	}
	e.taskQueue <- task
}

func (e *Engine) ensureRunContext(runID uuid.UUID) context.Context {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx, ok := e.runContexts[runID]; ok {
		return ctx
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.runContexts[runID] = ctx
	e.runCancels[runID] = cancel
	return ctx
}

func (e *Engine) cancelRunContext(runID uuid.UUID) {
	var cancel context.CancelFunc
	e.mu.Lock()
	cancel = e.runCancels[runID]
	delete(e.runCancels, runID)
	delete(e.runContexts, runID)
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (e *Engine) CancelRun(runID uuid.UUID) {
	e.mu.Lock()
	e.cancelledRuns[runID] = true
	e.mu.Unlock()

	e.cancelRunContext(runID)
	e.markRunRetriesCancelled(runID)

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
	if task.RetryID != "" {
		e.setRetryState(
			task.RetryID,
			task.RunID,
			workspaceID,
			task.AgentID,
			task.QuestionID,
			"running",
			uuid.Nil,
			"",
			0,
		)
	}

	if e.eventCallback != nil {
		payload := map[string]any{
			"run_id":      task.RunID.String(),
			"agent_id":    task.AgentID.String(),
			"question_id": task.QuestionID,
		}
		if task.RetryID != "" {
			payload["retry_id"] = task.RetryID
		}
		e.eventCallback(workspaceID, "EVT_TASK_STARTED", task.RunID.String(), payload)
	}

	runCtx := e.ensureRunContext(task.RunID)
	taskCtx, cancel := context.WithTimeout(runCtx, runnerTaskTimeout)
	defer cancel()

	startTime := time.Now().UTC()
	progressStop := make(chan struct{})
	if e.eventCallback != nil && workspaceID != uuid.Nil {
		go func(runID uuid.UUID, agentID uuid.UUID, questionID string) {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-progressStop:
					return
				case <-taskCtx.Done():
					return
				case <-ticker.C:
					elapsed := time.Since(startTime)
					payload := map[string]any{
						"run_id":      runID.String(),
						"agent_id":    agentID.String(),
						"question_id": questionID,
						"elapsed_ms":  int(elapsed.Milliseconds()),
						"message":     fmt.Sprintf("Runner still processing (%ds)", int(elapsed.Seconds())),
					}
					if task.RetryID != "" {
						payload["retry_id"] = task.RetryID
					}
					e.eventCallback(workspaceID, "EVT_TASK_PROGRESS", runID.String(), payload)
				}
			}
		}(task.RunID, task.AgentID, task.QuestionID)
	}
	defer close(progressStop)

	providerType := task.ProviderType
	if providerType == "evaluator" {
		providerType = ResolveEvaluatorProvider(task.AgentConfig)
	}

	payload := map[string]any{
		"question":          task.QuestionText,
		"expected_answer":   task.ExpectedAnswer,
		"original_question": task.OriginalQuestion,
		"agent_answer":      task.AgentAnswer, // For evaluators: the response to evaluate
		"answer":            task.AgentAnswer, // Alias for clarity in evaluator payloads
		"response":          task.AgentAnswer, // Extra alias for clarity in evaluator payloads
		"is_evaluator_task": task.ProviderType == "evaluator" || task.TargetRunResultID != uuid.Nil || strings.HasPrefix(strings.TrimSpace(task.QuestionID), "eval-"),
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

	executionResult, err := e.runner.Execute(taskCtx, req)
	if err != nil {
		executionResult = ExecutionResponse{Success: false, Error: err.Error()}
	}

	if !executionResult.Success {
		metaSummary := ""
		if len(executionResult.Metadata) > 0 {
			if payload, err := json.Marshal(executionResult.Metadata); err == nil {
				metaSummary = string(payload)
			} else {
				metaSummary = fmt.Sprintf("metadata_marshal_error=%v", err)
			}
		}
		if metaSummary != "" {
			log.Printf("[ENGINE] Task failed: Run %s, Agent %s, Question %s, Error=%s, Metadata=%s",
				task.RunID, task.AgentID, task.QuestionID, executionResult.Error, truncate(metaSummary, 2000))
		} else {
			log.Printf("[ENGINE] Task failed: Run %s, Agent %s, Question %s, Error=%s",
				task.RunID, task.AgentID, task.QuestionID, executionResult.Error)
		}
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

		if status == "success" && strings.TrimSpace(result.Answer) != "" {
			if err := e.persistEvaluatorScore(task, result.Answer); err != nil {
				log.Printf("[EVAL] Failed to persist automatic evaluator score for run %s, question %s: %v", task.RunID, task.QuestionID, err)
			}
		}

		// Auto-run evaluators for primary-agent retries only, scoped to the fresh result.
		// This avoids re-evaluating the whole run after a single answer retry.
		isPrimaryRetry := task.RetryID != "" &&
			strings.ToLower(strings.TrimSpace(task.ProviderType)) != "evaluator" &&
			!strings.HasPrefix(strings.TrimSpace(task.QuestionID), "eval-")
		if isPrimaryRetry && status == "success" && strings.TrimSpace(result.Answer) != "" {
			selectedEvaluatorIDs, selectErr := e.selectedEvaluatorIDsForRunID(task.RunID)
			if selectErr != nil {
				log.Printf("[EVAL] Failed to resolve selected evaluators for retry auto-run on run %s: %v", task.RunID, selectErr)
			}
			if len(selectedEvaluatorIDs) > 0 {
				if err := e.RunEvaluatorsForResults(task.RunID, selectedEvaluatorIDs, []models.RunResult{result}); err != nil {
					// Keep retry flow resilient; evaluator automation must not fail the primary retry.
					if !strings.Contains(strings.ToLower(err.Error()), "no evaluator agents available") {
						log.Printf("[EVAL] Auto-run after retry failed for run %s, question %s: %v", task.RunID, task.QuestionID, err)
					}
				}
			}
		}

		// Check if all tasks for this run are complete
		e.checkRunCompletion(task.RunID)
	}

	// Send task completed event
	if task.RetryID != "" {
		finalStatus := "completed"
		if !executionResult.Success {
			finalStatus = "error"
		}
		e.setRetryState(
			task.RetryID,
			task.RunID,
			workspaceID,
			task.AgentID,
			task.QuestionID,
			finalStatus,
			runResultID,
			executionResult.Error,
			durationMs,
		)
	}

	if e.eventCallback != nil {
		payload := map[string]any{
			"run_id":        task.RunID.String(),
			"run_result_id": runResultID.String(),
			"agent_id":      task.AgentID.String(),
			"question_id":   task.QuestionID,
			"success":       executionResult.Success,
			"answer":        executionResult.Answer,
			"error":         executionResult.Error,
			"metadata":      executionResult.Metadata,
			"duration_ms":   durationMs,
		}
		if task.TargetRunResultID != uuid.Nil {
			payload["target_run_result_id"] = task.TargetRunResultID.String()
		}
		if task.RetryID != "" {
			payload["retry_id"] = task.RetryID
		}
		e.eventCallback(workspaceID, "EVT_TASK_COMPLETED", task.RunID.String(), payload)
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

	if run.Status == "cancelled" {
		return
	}

	// Count expected vs completed results
	var count int64
	e.db.Model(&models.RunResult{}).Where("run_id = ?", runID).Count(&count)

	if int(count) >= run.TotalTasks {
		// Auto-run evaluators once, right after primary tasks complete.
		// We only do this while run is in "running" state and before any evaluator
		// result exists, so retries/manual evaluator runs won't loop re-queueing.
		if strings.EqualFold(strings.TrimSpace(run.Status), "running") {
			var evalResultCount int64
			if err := e.db.Model(&models.RunResult{}).
				Where("run_id = ? AND question_id LIKE ?", runID, "eval-%").
				Count(&evalResultCount).Error; err == nil && evalResultCount == 0 {
				selectedEvaluatorIDs, selectErr := e.selectedEvaluatorIDsForRun(run)
				if selectErr != nil {
					log.Printf("[EVAL] Failed to resolve selected evaluators for run %s: %v", runID, selectErr)
				}
				if len(selectedEvaluatorIDs) > 0 {
					previousTotalTasks := run.TotalTasks
					if err := e.RunEvaluators(runID, selectedEvaluatorIDs); err != nil {
						if !strings.Contains(strings.ToLower(err.Error()), "no evaluator agents available") {
							log.Printf("[EVAL] Auto-run failed for run %s: %v", runID, err)
						}
					} else {
						var refreshed models.Run
						if refreshErr := e.db.First(&refreshed, "id = ?", runID).Error; refreshErr == nil {
							if refreshed.TotalTasks > previousTotalTasks {
								log.Printf("[EVAL] Auto-run queued for run %s (%d -> %d total tasks)", runID, previousTotalTasks, refreshed.TotalTasks)
								return
							}
							run = refreshed
						}
					}
				}
			}
		}

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

		e.cancelRunContext(runID)
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

func extractEvaluatorScore(answer string) (int, bool) {
	text := strings.TrimSpace(answer)
	if text == "" {
		return 0, false
	}

	if m := evaluatorScoreAtEndRegex.FindStringSubmatch(text); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 && n <= 10 {
			return n, true
		}
	}

	all := evaluatorScoreAnyRegex.FindAllStringSubmatch(text, -1)
	for i := len(all) - 1; i >= 0; i-- {
		if len(all[i]) < 3 {
			continue
		}
		n, err := strconv.Atoi(all[i][2])
		if err != nil {
			continue
		}
		if n >= 0 && n <= 10 {
			return n, true
		}
	}

	return 0, false
}

func mapEvaluatorScore(score10 int) (rating string, ratingCode int, score100 int) {
	if score10 < 0 {
		score10 = 0
	}
	if score10 > 10 {
		score10 = 10
	}

	score100 = score10 * 10

	switch {
	case score10 >= 8:
		return "like", 1, score100
	case score10 >= 6:
		return "valid", 2, score100
	default:
		return "dislike", 3, score100
	}
}

func isEvaluatorExecutionTask(task *Task) bool {
	if task == nil {
		return false
	}
	if task.TargetRunResultID != uuid.Nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(task.ProviderType), "evaluator") {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(task.QuestionID), "eval-")
}

func (e *Engine) resolveEvaluatorTargetRunResultID(task *Task) uuid.UUID {
	if e.db == nil || task == nil {
		return uuid.Nil
	}
	if task.TargetRunResultID != uuid.Nil {
		return task.TargetRunResultID
	}

	targetAgentID, targetQuestionID := extractTargetAgentID(strings.TrimSpace(task.QuestionID))
	if targetAgentID == uuid.Nil || strings.TrimSpace(targetQuestionID) == "" {
		return uuid.Nil
	}

	var target models.RunResult
	if err := e.db.
		Where("run_id = ? AND agent_id = ? AND question_id = ? AND status = ?",
			task.RunID, targetAgentID, targetQuestionID, "success").
		Order("created_at DESC").
		Order("id DESC").
		First(&target).Error; err != nil {
		return uuid.Nil
	}

	return target.ID
}

func (e *Engine) persistEvaluatorScore(task *Task, evaluatorAnswer string) error {
	if !isEvaluatorExecutionTask(task) || e.db == nil || task == nil {
		return nil
	}

	targetResultID := e.resolveEvaluatorTargetRunResultID(task)
	if targetResultID == uuid.Nil {
		return nil
	}

	score10, ok := extractEvaluatorScore(evaluatorAnswer)
	if !ok {
		return nil
	}

	rating, ratingCode, score100 := mapEvaluatorScore(score10)

	if err := e.db.
		Where("run_result_id = ? AND rater_type = ? AND rater_id = ?", targetResultID, "agent", task.AgentID).
		Delete(&models.Evaluation{}).Error; err != nil {
		return err
	}

	eval := models.Evaluation{
		ID:          uuid.New(),
		RunResultID: targetResultID,
		RaterType:   "agent",
		RaterID:     task.AgentID,
		Rating:      rating,
		RatingCode:  &ratingCode,
		Score:       &score100,
		Comments:    strings.TrimSpace(evaluatorAnswer),
	}

	return e.db.Create(&eval).Error
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

func validateAgentSetComposition(agents []models.Agent) error {
	total := len(agents)
	if total == 0 {
		return fmt.Errorf("question set must include at least one primary agent")
	}
	if total > 2 {
		return fmt.Errorf("question set can include at most 2 agents")
	}

	primaryCount := 0
	evaluatorCount := 0
	for _, agent := range agents {
		if isEvaluatorAgent(agent) {
			evaluatorCount++
		} else {
			primaryCount++
		}
	}

	if evaluatorCount > 1 {
		return fmt.Errorf("question set can include at most 1 evaluator agent")
	}
	if primaryCount == 0 {
		return fmt.Errorf("question set must include at least one primary agent (evaluator-only sets are not allowed)")
	}
	if primaryCount > 2 {
		return fmt.Errorf("question set can include at most 2 primary agents")
	}
	if evaluatorCount == 1 && primaryCount != 1 {
		return fmt.Errorf("question set with evaluator must include exactly 1 primary agent")
	}

	return nil
}

func (e *Engine) loadEnabledQuestionSetAgents(questionSetID uuid.UUID) ([]models.Agent, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var links []models.QuestionSetAgent
	if err := e.db.Preload("Agent").
		Where("question_set_id = ? AND enabled = true", questionSetID).
		Order("position ASC").
		Find(&links).Error; err != nil {
		return nil, err
	}

	selected := make([]models.Agent, 0, len(links))
	for _, link := range links {
		if link.Agent.ID == uuid.Nil {
			continue
		}
		agent := link.Agent
		agent.Enabled = true
		agent.Position = link.Position
		if len(link.Config) > 0 {
			agent.Config = link.Config
		}
		selected = append(selected, agent)
	}

	return selected, nil
}

func (e *Engine) selectedEvaluatorIDsForRun(run models.Run) ([]uuid.UUID, error) {
	selectedAgents, err := e.loadEnabledQuestionSetAgents(run.QuestionSetID)
	if err != nil {
		return nil, err
	}
	if len(selectedAgents) == 0 {
		return nil, nil
	}
	if err := validateAgentSetComposition(selectedAgents); err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, 1)
	for _, agent := range selectedAgents {
		if isEvaluatorAgent(agent) {
			ids = append(ids, agent.ID)
		}
	}
	return ids, nil
}

func (e *Engine) selectedEvaluatorIDsForRunID(runID uuid.UUID) ([]uuid.UUID, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return nil, err
	}

	return e.selectedEvaluatorIDsForRun(run)
}

func (e *Engine) queueEvaluatorTasksForResults(run models.Run, results []models.RunResult, selectedEvaluatorIDs []uuid.UUID) error {
	if e.db == nil {
		return fmt.Errorf("database not configured")
	}

	// Get evaluator candidates (native evaluator + legacy openai evaluators)
	var evaluatorCandidates []models.Agent
	if err := e.db.Where("workspace_id = ? AND provider_type IN ? AND enabled = true", run.WorkspaceID, []string{"evaluator", "openai"}).Find(&evaluatorCandidates).Error; err != nil {
		return err
	}

	selectedSet := make(map[uuid.UUID]struct{})
	for _, id := range selectedEvaluatorIDs {
		if id != uuid.Nil {
			selectedSet[id] = struct{}{}
		}
	}
	if len(selectedSet) == 0 {
		return fmt.Errorf("no evaluator agents selected")
	}

	evaluators := make([]models.Agent, 0, len(evaluatorCandidates))
	for _, candidate := range evaluatorCandidates {
		if !isEvaluatorAgent(candidate) {
			continue
		}
		if _, ok := selectedSet[candidate.ID]; !ok {
			continue
		}
		evaluators = append(evaluators, candidate)
	}
	if len(evaluators) == 0 {
		return fmt.Errorf("no evaluator agents available")
	}

	// Validate evaluator configurations
	for _, eval := range evaluators {
		if err := e.validateEvaluatorConfig(eval); err != nil {
			return err
		}
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

	log.Printf("[EVAL] Running evaluators for run %s. Results to evaluate: %d", run.ID, len(results))

	tasksToQueue := make([]*Task, 0)

	// For each evaluator, create tasks to evaluate each result
	for _, evaluator := range evaluators {
		baseEvalConfig := decodeConfigJSON(evaluator.Config)
		evalConfig := baseEvalConfig

		// Check for question set override
		var override models.QuestionSetAgent
		if err := e.db.Where("question_set_id = ? AND agent_id = ?", run.QuestionSetID, evaluator.ID).First(&override).Error; err == nil {
			if !override.Enabled {
				continue // Skip if explicitly disabled for this question set
			}
			if len(override.Config) > 0 {
				overrideCfg := decodeConfigJSON(override.Config)
				evalConfig = mergeConfig(baseEvalConfig, overrideCfg)
			}
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
				RunID:             run.ID,
				WorkspaceID:       run.WorkspaceID,
				AgentID:           evaluator.ID,
				QuestionID:        fmt.Sprintf("eval-%s-%s", result.AgentID, questionID),
				QuestionText:      evalQuestion,
				ExpectedAnswer:    expectedAnswer,
				OriginalQuestion:  originalQuestion,
				AgentAnswer:       result.Answer, // Explicit agent answer for Python server
				AgentConfig:       evalConfig,
				ProviderType:      evaluator.ProviderType,
				MaxConcurrency:    evaluator.MaxConcurrency,
				TargetRunResultID: result.ID,
			}
			tasksToQueue = append(tasksToQueue, task)
		}
	}

	if len(tasksToQueue) == 0 {
		log.Printf("[EVAL] No evaluator tasks to queue for run %s", run.ID)
		return nil
	}

	if err := e.db.Model(&models.Run{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status":      "running",
		"total_tasks": gorm.Expr("total_tasks + ?", len(tasksToQueue)),
	}).Error; err != nil {
		return err
	}

	e.ensureRunContext(run.ID)
	for _, task := range tasksToQueue {
		e.QueueTask(task)
	}
	log.Printf("[EVAL] Queued %d evaluator tasks for run %s", len(tasksToQueue), run.ID)

	return nil
}

// RunEvaluators runs evaluator agents on all eligible primary results of a run.
func (e *Engine) RunEvaluators(runID uuid.UUID, selectedEvaluatorIDs []uuid.UUID) error {
	if e.db == nil {
		return fmt.Errorf("database not configured")
	}

	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return err
	}
	if run.Status == "cancelled" {
		return fmt.Errorf("run is cancelled")
	}

	var results []models.RunResult
	if err := e.db.Where("run_id = ?", runID).Find(&results).Error; err != nil {
		return err
	}

	return e.queueEvaluatorTasksForResults(run, results, selectedEvaluatorIDs)
}

// RunEvaluatorsForResults runs evaluator agents only for the provided result subset.
func (e *Engine) RunEvaluatorsForResults(runID uuid.UUID, selectedEvaluatorIDs []uuid.UUID, results []models.RunResult) error {
	if e.db == nil {
		return fmt.Errorf("database not configured")
	}
	if len(results) == 0 {
		return nil
	}

	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return err
	}
	if run.Status == "cancelled" {
		return fmt.Errorf("run is cancelled")
	}

	filtered := make([]models.RunResult, 0, len(results))
	for _, r := range results {
		if r.RunID != runID {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return nil
	}

	return e.queueEvaluatorTasksForResults(run, filtered, selectedEvaluatorIDs)
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
		e.db.Where("id IN ? AND workspace_id = ?", agentIDs, workspaceID).Find(&agents)
	} else {
		// Load all primary agents; question-set overrides become the source of truth when present.
		var workspaceAgents []models.Agent
		e.db.Where("workspace_id = ? AND provider_type != 'evaluator'", workspaceID).Order("position ASC").Find(&workspaceAgents)

		// When mappings exist, use only mapped and enabled agents for this question set.
		if len(overrides) > 0 {
			for _, wa := range workspaceAgents {
				if o, ok := overrideMap[wa.ID]; ok {
					if o.Enabled {
						agents = append(agents, wa)
					}
				}
			}
		} else {
			// Legacy fallback for old question sets with no mapping yet.
			for _, wa := range workspaceAgents {
				if wa.Enabled {
					agents = append(agents, wa)
				}
			}
		}
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}
	if err := validateAgentSetComposition(agents); err != nil {
		return nil, err
	}

	selectedSetAgents, err := e.loadEnabledQuestionSetAgents(questionSetID)
	if err != nil {
		return nil, err
	}
	if len(selectedSetAgents) > 0 {
		if err := validateAgentSetComposition(selectedSetAgents); err != nil {
			return nil, err
		}
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
		if !isEvaluatorAgent(agent) {
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

	e.ensureRunContext(run.ID)

	// Queue tasks for each agent + question
	for _, agent := range agents {
		if isEvaluatorAgent(agent) {
			log.Printf("[ENGINE] Skipping evaluator agent %s during initial StartRun", agent.ID)
			continue
		}
		baseAgentConfig := decodeConfigJSON(agent.Config)
		agentConfig := baseAgentConfig

		// Use override if available
		if override, ok := overrideMap[agent.ID]; ok && len(override.Config) > 0 {
			overrideCfg := decodeConfigJSON(override.Config)
			agentConfig = mergeConfig(baseAgentConfig, overrideCfg)
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
	RetryID          string
}

// RerunTask reruns a single question for a specific agent
func (e *Engine) RerunTask(runID uuid.UUID, agentID uuid.UUID, questionID string, opts *RerunTaskOptions) error {
	retryID := ""
	if opts != nil {
		retryID = strings.TrimSpace(opts.RetryID)
	}
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

	e.ensureRunContext(run.ID)

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
	taskTargetRunResultID := uuid.Nil

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
				taskTargetRunResultID = targetResult.ID
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
						if !isEvaluatorAgent(rAgent) {
							taskAgentAnswer = r.Answer
							taskQuestionText = r.Answer
							taskTargetRunResultID = r.ID
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
		RunID:             run.ID,
		WorkspaceID:       run.WorkspaceID,
		AgentID:           agent.ID,
		QuestionID:        questionID,
		QuestionText:      taskQuestionText,
		OriginalQuestion:  taskOriginalQuestion,
		ExpectedAnswer:    expectedAnswer,
		AgentAnswer:       taskAgentAnswer,
		AgentConfig:       agentConfig,
		ProviderType:      agent.ProviderType,
		MaxConcurrency:    agent.MaxConcurrency,
		RetryID:           retryID,
		TargetRunResultID: taskTargetRunResultID,
	}

	log.Printf("[RERUN] Queuing task: run=%s, agent=%s, qid=%s, ans_len=%d", task.RunID, task.AgentID, task.QuestionID, len(task.AgentAnswer))
	e.QueueTask(task)

	return nil
}

func parseAgentConfig(agent models.Agent) map[string]any {
	cfg := make(map[string]any)
	if len(agent.Config) == 0 {
		return cfg
	}
	if err := json.Unmarshal(agent.Config, &cfg); err != nil {
		return map[string]any{}
	}
	return cfg
}

func isLegacyEvaluatorConfig(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	if _, hasTarget := cfg["target_agent_id"]; hasTarget {
		return true
	}
	if mode := strings.TrimSpace(firstNonEmptyString(cfg, "openai_mode")); mode != "" {
		return true
	}
	if sys := strings.TrimSpace(firstNonEmptyString(cfg, "system_prompt", "instructions")); sys != "" {
		return true
	}
	return false
}

func isEvaluatorAgent(agent models.Agent) bool {
	switch strings.ToLower(strings.TrimSpace(agent.ProviderType)) {
	case "evaluator":
		return true
	case "openai":
		cfg := parseAgentConfig(agent)
		if isLegacyEvaluatorConfig(cfg) {
			return true
		}
		name := strings.ToLower(strings.TrimSpace(agent.Name))
		return strings.Contains(name, "evaluator")
	default:
		return false
	}
}

func (e *Engine) validateEvaluatorConfig(evaluator models.Agent) error {
	configStr := string(evaluator.Config)
	var config map[string]any
	if err := json.Unmarshal(evaluator.Config, &config); err != nil {
		return fmt.Errorf("invalid evaluator config for %s: %v", evaluator.Name, err)
	}

	preferredProvider := PreferredEvaluatorProvider(config)
	resolvedProvider := ResolveEvaluatorProvider(config)

	// If "MOCK" or "DRYRUN" is explicitly present anywhere in the config (case insensitive), we allow it
	upperConfig := strings.ToUpper(configStr)
	if strings.Contains(upperConfig, "MOCK") || strings.Contains(upperConfig, "DRYRUN") {
		return nil
	}

	// Otherwise, it must be a real configuration for the selected provider.
	if IsSelectedEvaluatorProviderConfigured(config) {
		return nil
	}

	switch preferredProvider {
	case EvaluatorProviderNVIDIA:
		return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set nvidia_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
	case EvaluatorProviderOpenRouter:
		return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set openrouter_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
	case EvaluatorProviderAnthropic:
		return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set anthropic_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
	case EvaluatorProviderOpenAICompatible:
		return fmt.Errorf("Evaluator Agent '%s' is not configured. OpenAI-compatible mode requires compatible_api_key and compatible_base_url (or legacy api_key/base_url).", evaluator.Name)
	case EvaluatorProviderAuto:
		return fmt.Errorf("Evaluator Agent '%s' is not configured. In auto mode, configure at least one provider: nvidia_api_key, openrouter_api_key, anthropic_api_key, openai_api_key, or compatible_api_key+compatible_base_url.", evaluator.Name)
	default:
		if resolvedProvider == EvaluatorProviderNVIDIA {
			return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set nvidia_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
		}
		if resolvedProvider == EvaluatorProviderOpenRouter {
			return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set openrouter_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
		}
		if resolvedProvider == EvaluatorProviderAnthropic {
			return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set anthropic_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
		}
		if resolvedProvider == EvaluatorProviderOpenAICompatible {
			return fmt.Errorf("Evaluator Agent '%s' is not configured. OpenAI-compatible mode requires compatible_api_key and compatible_base_url (or legacy api_key/base_url).", evaluator.Name)
		}
		if EvaluatorOpenAIMode(config) == "managed" {
			return fmt.Errorf("Evaluator Agent '%s' is not configured. OpenAI managed mode requires openai_api_key (or api_key) and prompt_id/openai_prompt_id.", evaluator.Name)
		}
		return fmt.Errorf("Evaluator Agent '%s' is not configured. Please set openai_api_key (or legacy api_key) in Agent Settings, or use 'MOCK' for testing.", evaluator.Name)
	}
}
