package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"benchmarking-platform/models"
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
	AgentID          uuid.UUID
	QuestionID       string
	QuestionText     string
	ExpectedAnswer   string
	OriginalQuestion string
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

	startTime := time.Now()

	providerType := task.ProviderType
	if providerType == "evaluator" {
		providerType = "openai"
	}

	req := ExecutionRequest{
		RequestID:    uuid.New().String(),
		ProviderType: providerType,
		Config:       task.AgentConfig,
		Payload: map[string]any{
			"question":          task.QuestionText,
			"expected_answer":   task.ExpectedAnswer,
			"original_question": task.OriginalQuestion,
		},
	}

	body, _ := json.Marshal(req)

	resp, err := e.httpClient.Post(fmt.Sprintf("%s/execute", e.pythonRunnerURL), "application/json", bytes.NewBuffer(body))

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

	var qsData struct {
		Categories []struct {
			Questions []struct {
				ID       any    `json:"id"`
				Question string `json:"question"`
				Expected string `json:"expected"`
			} `json:"questions"`
		} `json:"categories"`
	}
	json.Unmarshal(qs.Data, &qsData)

	// Create a map for quick lookup of expected answers and questions
	expectedAnswers := make(map[string]string)
	originalQuestions := make(map[string]string)
	for _, cat := range qsData.Categories {
		for _, q := range cat.Questions {
			qID := ""
			switch v := q.ID.(type) {
			case string:
				qID = v
			case float64:
				qID = fmt.Sprintf("%.0f", v)
			}
			expectedAnswers[qID] = q.Expected
			originalQuestions[qID] = q.Question
		}
	}

	// For each evaluator, create tasks to evaluate each result
	for _, evaluator := range evaluators {
		var evalConfig map[string]any
		json.Unmarshal(evaluator.Config, &evalConfig)

		// Get target agent ID from evaluator config
		targetAgentIDStr, _ := evalConfig["target_agent_id"].(string)
		targetAgentID, _ := uuid.Parse(targetAgentIDStr)

		for _, result := range results {
			// Only evaluate results from the target agent
			if targetAgentID != uuid.Nil && result.AgentID != targetAgentID {
				continue
			}

			// Build evaluation prompt (now delegating more context to the worker)
			evalQuestion := result.Answer // The answer we are evaluating

			task := &Task{
				RunID:            runID,
				AgentID:          evaluator.ID,
				QuestionID:       fmt.Sprintf("eval-%s-%s", result.AgentID, result.QuestionID),
				QuestionText:     evalQuestion,
				ExpectedAnswer:   expectedAnswers[result.QuestionID],
				OriginalQuestion: originalQuestions[result.QuestionID],
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
	for _, cat := range qsData.Categories {
		totalTasks += len(cat.Questions)
	}
	totalTasks = totalTasks * len(agents)

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

// RerunTask reruns a single question for a specific agent
func (e *Engine) RerunTask(runID uuid.UUID, agentID uuid.UUID, questionID string) error {
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

	// Get question set to find the question text
	var questionSet models.QuestionSet
	if err := e.db.First(&questionSet, "id = ?", run.QuestionSetID).Error; err != nil {
		return fmt.Errorf("question set not found")
	}

	var qsData struct {
		Categories []struct {
			Questions []struct {
				ID       any    `json:"id"`
				Question string `json:"question"`
			} `json:"questions"`
		} `json:"categories"`
	}
	json.Unmarshal(questionSet.Data, &qsData)

	// Find the question text
	var questionText string
	for _, cat := range qsData.Categories {
		for _, q := range cat.Questions {
			qID := ""
			switch v := q.ID.(type) {
			case string:
				qID = v
			case float64:
				qID = fmt.Sprintf("%.0f", v)
			}
			if qID == questionID {
				questionText = q.Question
				break
			}
		}
	}

	if questionText == "" {
		return fmt.Errorf("question not found")
	}

	var agentConfig map[string]any
	json.Unmarshal(agent.Config, &agentConfig)

	task := &Task{
		RunID:          run.ID,
		AgentID:        agent.ID,
		QuestionID:     questionID,
		QuestionText:   questionText,
		AgentConfig:    agentConfig,
		ProviderType:   agent.ProviderType,
		MaxConcurrency: agent.MaxConcurrency,
	}
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
