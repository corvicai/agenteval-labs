package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"
)

// isEvaluatorRunResult returns true when a RunResult represents an evaluator
// task rather than a primary agent answer. Evaluator RunResults are stored
// with QuestionID formatted as "eval-<targetAgentID>-<originalQuestionID>"
// and carry no evaluations of their own (evaluations are attached to the
// primary RunResult via the Evaluation relation). Excluding them avoids
// double-counting in comparison reports (e.g. 135 questions + 135 evaluator
// rows would otherwise surface as "272 questions").
func isEvaluatorRunResult(r models.RunResult) bool {
	return strings.HasPrefix(strings.TrimSpace(r.QuestionID), "eval-")
}

// ---------- Types ----------

type compareRunsRequest struct {
	RunIDs         []uuid.UUID       `json:"run_ids"`
	Labels         map[string]string `json:"labels"`          // run_id -> label
	MetricsEnabled map[string]bool   `json:"metrics_enabled"` // keys: totals, agent_scores, latency, success_quality, per_question, regressions
}

type ComparisonRunBlock struct {
	ID          uuid.UUID              `json:"id"`
	Label       string                 `json:"label"`
	Name        string                 `json:"name"`
	QuestionSet ComparisonQSRef        `json:"question_set"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	Totals      ComparisonTotals       `json:"totals"`
	Agents      []ComparisonAgentBlock `json:"agents"`
	PerQuestion []ComparisonQScore     `json:"per_question"`
}

type ComparisonQSRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ComparisonTotals struct {
	Questions  int   `json:"questions"`
	Completed  int   `json:"completed"`
	Errors     int   `json:"errors"`
	DurationMs int64 `json:"duration_ms"`
}

type ComparisonAgentBlock struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	AvgScore     float64   `json:"avg_score"`
	SuccessRate  float64   `json:"success_rate"`
	QualityRate  float64   `json:"quality_rate"`
	AvgLatencyMs float64   `json:"avg_latency_ms"`
	EvalsCount   int       `json:"evals_count"`
	ResultsCount int       `json:"results_count"`
}

type ComparisonQScore struct {
	QuestionID string    `json:"question_id"`
	AgentID    uuid.UUID `json:"agent_id"`
	Score      *float64  `json:"score,omitempty"`  // avg of numeric scores
	Rating     string    `json:"rating,omitempty"` // dominant rating
	HasError   bool      `json:"has_error"`
}

type ComparisonRegression struct {
	QuestionID string    `json:"question_id"`
	AgentID    uuid.UUID `json:"agent_id"`
	FromLabel  string    `json:"from_label"`
	ToLabel    string    `json:"to_label"`
	FromScore  *float64  `json:"from_score,omitempty"`
	ToScore    *float64  `json:"to_score,omitempty"`
	Delta      float64   `json:"delta"`
}

type ComparisonReport struct {
	SameQuestionSet   bool                   `json:"same_question_set"`
	SameAgents        bool                   `json:"same_agents"`
	CommonQuestionIDs []string               `json:"common_question_ids"`
	Runs              []ComparisonRunBlock   `json:"runs"`
	Regressions       []ComparisonRegression `json:"regressions"`
	MetricsEnabled    map[string]bool        `json:"metrics_enabled"`
	GeneratedAt       time.Time              `json:"generated_at"`
}

// ---------- Handlers ----------

func (h *Hub) handleCompareRuns(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "unauthorized")
		return
	}

	var req compareRunsRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		c.SendError(env.CorrelationID, "bad_request: invalid payload")
		return
	}

	if len(req.RunIDs) < 2 {
		c.SendError(env.CorrelationID, "bad_request: at least 2 run_ids required")
		return
	}

	report, err := h.buildComparisonReport(context.Background(), c.WorkspaceID, c.UserID, req)
	if err != nil {
		logger.Error("Failed to build comparison report: %v", err)
		c.SendError(env.CorrelationID, "internal: "+err.Error())
		return
	}

	c.SendResponse(ResCompareRuns, env.CorrelationID, report)
}

func (h *Hub) handleCreateComparison(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleListComparisons(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleGetComparison(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleDeleteComparison(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleListComparisonTemplates(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleCreateComparisonTemplate(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleUpdateComparisonTemplate(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

func (h *Hub) handleDeleteComparisonTemplate(c *Connection, env models.Envelope) {
	c.SendError(env.CorrelationID, "not_implemented")
}

// ---------- Core Logic ----------

func (h *Hub) buildComparisonReport(ctx context.Context, workspaceID, userID uuid.UUID, req compareRunsRequest) (*ComparisonReport, error) {
	// 1. Fetch runs the caller may read: runs in their own workspace, or runs on
	// a question set actively shared with them (accepted, not revoked).
	var runs []models.Run
	if err := h.db.Preload("QuestionSet").
		Preload("Results").
		Preload("Results.Evaluations").
		Where("id IN ? AND (workspace_id = ? OR question_set_id IN (SELECT question_set_id FROM question_set_collaborators WHERE user_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL))", req.RunIDs, workspaceID, userID).
		Find(&runs).Error; err != nil {
		return nil, err
	}

	// Find all runs by IDs.
	if len(runs) == 0 {
		return nil, errors.New("no runs found")
	}

	runsMap := make(map[uuid.UUID]models.Run)
	for _, r := range runs {
		runsMap[r.ID] = r
	}
	
	sortedRuns := make([]models.Run, 0, len(req.RunIDs))
	for _, id := range req.RunIDs {
		r, ok := runsMap[id]
		if !ok {
			return nil, fmt.Errorf("run not found or access denied: %s", id)
		}
		sortedRuns = append(sortedRuns, r)
	}

	report := &ComparisonReport{
		SameQuestionSet:   true,
		SameAgents:        true,
		Runs:              make([]ComparisonRunBlock, 0, len(sortedRuns)),
		Regressions:       []ComparisonRegression{},
		MetricsEnabled:    req.MetricsEnabled,
		GeneratedAt:       time.Now(),
	}

	var firstQS uuid.UUID
	var commonQuestions map[string]int
	agentCount := make(map[uuid.UUID]int)

	// Fetch agent names to avoid N+1 and because RunResult misses Agent relation.
	// Only collect IDs from primary (non-evaluator) results so the radar/latency
	// charts compare primary agent performance without evaluator agents
	// appearing as separate series.
	var agentIDs []uuid.UUID
	for _, r := range sortedRuns {
		for _, res := range r.Results {
			if isEvaluatorRunResult(res) {
				continue
			}
			agentIDs = append(agentIDs, res.AgentID)
		}
	}
	var agents []models.Agent
	agentNames := make(map[uuid.UUID]string)
	if len(agentIDs) > 0 {
		h.db.Where("id IN ?", agentIDs).Find(&agents)
		for _, a := range agents {
			agentNames[a.ID] = a.Name
		}
	}

	commonQuestionsInit := false

	for i, r := range sortedRuns {
		label := req.Labels[r.ID.String()]
		if label == "" {
			label = fmt.Sprintf("Run %d", i+1)
		}

		if i == 0 {
			firstQS = r.QuestionSetID
		} else if firstQS != r.QuestionSetID {
			report.SameQuestionSet = false
		}

		qsName := ""
		if r.QuestionSet != nil {
			qsName = r.QuestionSet.Name
		} else {
			qsName = r.QuestionSetName
		}

		runBlock := ComparisonRunBlock{
			ID:    r.ID,
			Label: label,
			Name:  fmt.Sprintf("Run %s", r.ID.String()[:8]),
			QuestionSet: ComparisonQSRef{
				ID:   r.QuestionSetID,
				Name: qsName,
			},
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			Totals:      ComparisonTotals{},
			Agents:      []ComparisonAgentBlock{},
			PerQuestion: []ComparisonQScore{},
		}

		// Calculate statistics. We iterate PRIMARY results only (evaluator
		// RunResults use QuestionID="eval-..." and carry the evaluator agent
		// as AgentID, with no Evaluations of their own). The real evaluator
		// ratings live on the primary result's Evaluations slice, already
		// preloaded in the query above.
		agentsData := make(map[uuid.UUID]*ComparisonAgentBlock)
		questionsInRun := make(map[string]bool)

		for _, res := range r.Results {
			if isEvaluatorRunResult(res) {
				continue
			}
			questionsInRun[res.QuestionID] = true

			if _, ok := agentsData[res.AgentID]; !ok {
				agentName := "Unknown"
				if name, ok := agentNames[res.AgentID]; ok {
					agentName = name
				}
				agentsData[res.AgentID] = &ComparisonAgentBlock{
					ID:   res.AgentID,
					Name: agentName,
				}
				agentCount[res.AgentID]++
			}

			aData := agentsData[res.AgentID]
			aData.ResultsCount++
			if res.Status == "success" {
				aData.SuccessRate++
			} else {
				runBlock.Totals.Errors++
			}
			aData.AvgLatencyMs += float64(res.DurationMs)
			runBlock.Totals.DurationMs += int64(res.DurationMs)

			qScore := ComparisonQScore{
				QuestionID: res.QuestionID,
				AgentID:    res.AgentID,
				HasError:   res.Status != "success",
			}

			// Evaluations
			if len(res.Evaluations) > 0 {
				var sumScore float64
				var validScores int
				likeCnt, validCnt, dislikeCnt, wrongCnt := 0, 0, 0, 0

				for _, eval := range res.Evaluations {
					aData.EvalsCount++

					// Quality rate (likes / valid)
					if eval.Rating == "like" || eval.Rating == "valid" {
						aData.QualityRate++
					}

					// Score Numeric Map
					// 1=like, 2=valid, 3=dislike, 4=wrong
					if eval.Score != nil {
						sumScore += float64(*eval.Score)
						validScores++
					} else {
						// Map rating_code to 1-5 scale: like=5, valid=4, partial=3, dislike=2, wrong=1
						var s float64
						switch eval.Rating {
						case "like":
							s = 5
							likeCnt++
						case "valid":
							s = 4
							validCnt++
						case "partial":
							s = 3
						case "dislike":
							s = 2
							dislikeCnt++
						case "wrong":
							s = 1
							wrongCnt++
						}
						if s > 0 {
							sumScore += s
							validScores++
						}
					}
				}

				if validScores > 0 {
					avgS := sumScore / float64(validScores)
					qScore.Score = &avgS
				}

				// Dominant rating
				if likeCnt >= validCnt && likeCnt >= dislikeCnt && likeCnt >= wrongCnt && likeCnt > 0 {
					qScore.Rating = "like"
				} else if validCnt >= dislikeCnt && validCnt >= wrongCnt && validCnt > 0 {
					qScore.Rating = "valid"
				} else if dislikeCnt >= wrongCnt && dislikeCnt > 0 {
					qScore.Rating = "dislike"
				} else if wrongCnt > 0 {
					qScore.Rating = "wrong"
				}

			}

			runBlock.PerQuestion = append(runBlock.PerQuestion, qScore)
		}

		// Normalize averages
		for _, aData := range agentsData {
			if aData.ResultsCount > 0 {
				aData.SuccessRate = (aData.SuccessRate / float64(aData.ResultsCount)) * 100
				aData.AvgLatencyMs = aData.AvgLatencyMs / float64(aData.ResultsCount)
			}
			if aData.EvalsCount > 0 {
				aData.QualityRate = (aData.QualityRate / float64(aData.EvalsCount)) * 100
			}

			// Avg score per agent -> compute based on per_question scores
			var tScore float64
			var cnt int
			for _, pq := range runBlock.PerQuestion {
				if pq.AgentID == aData.ID && pq.Score != nil {
					tScore += *pq.Score
					cnt++
				}
			}
			if cnt > 0 {
				aData.AvgScore = tScore / float64(cnt)
			}

			runBlock.Agents = append(runBlock.Agents, *aData)
		}

		// Totals.Questions is the number of distinct primary questions the
		// run attempted. Using r.TotalTasks would double-count because it
		// includes evaluator tasks queued by the orchestrator.
		runBlock.Totals.Questions = len(questionsInRun)
		runBlock.Totals.Completed = runBlock.Totals.Questions - runBlock.Totals.Errors
		if runBlock.Totals.Completed < 0 {
			runBlock.Totals.Completed = 0
		}

		report.Runs = append(report.Runs, runBlock)

		// Intersection of questions
		if !commonQuestionsInit {
			commonQuestions = make(map[string]int)
			for q := range questionsInRun {
				commonQuestions[q] = 1
			}
			commonQuestionsInit = true
		} else {
			for q := range questionsInRun {
				if _, exists := commonQuestions[q]; exists {
					commonQuestions[q]++
				}
			}
		}
	}

	// Filter common questions
	for q, cnt := range commonQuestions {
		if cnt == len(sortedRuns) {
			report.CommonQuestionIDs = append(report.CommonQuestionIDs, q)
		}
	}

	// Check same agents
	expectedAgentCount := len(sortedRuns)
	for _, cnt := range agentCount {
		if cnt != expectedAgentCount {
			report.SameAgents = false
			break
		}
	}

	// Detect Regressions
	if req.MetricsEnabled["regressions"] {
		report.Regressions = detectRegressions(report.Runs)
	}

	return report, nil
}

func detectRegressions(runs []ComparisonRunBlock) []ComparisonRegression {
	var regressions []ComparisonRegression
	if len(runs) < 2 {
		return regressions
	}

	for i := 0; i < len(runs)-1; i++ {
		r1 := runs[i]
		r2 := runs[i+1]

		qMap1 := make(map[string]map[uuid.UUID]*float64)
		for _, pq := range r1.PerQuestion {
			if _, ok := qMap1[pq.QuestionID]; !ok {
				qMap1[pq.QuestionID] = make(map[uuid.UUID]*float64)
			}
			qMap1[pq.QuestionID][pq.AgentID] = pq.Score
		}

		for _, pq2 := range r2.PerQuestion {
			if pq1Scores, hasQ := qMap1[pq2.QuestionID]; hasQ {
				if pq1Score, hasA := pq1Scores[pq2.AgentID]; hasA {
					if pq1Score != nil && pq2.Score != nil {
						delta := *pq2.Score - *pq1Score
						if delta <= -1.0 { // Threshold
							regressions = append(regressions, ComparisonRegression{
								QuestionID: pq2.QuestionID,
								AgentID:    pq2.AgentID,
								FromLabel:  r1.Label,
								ToLabel:    r2.Label,
								FromScore:  pq1Score,
								ToScore:    pq2.Score,
								Delta:      delta,
							})
						}
					}
				}
			}
		}
	}

	return regressions
}

func computeRunsSnapshotHash(runs []models.Run) string {
	type entry struct {
		ID         uuid.UUID
		Status     string
		TotalTasks int
		ResultsLen int
		UpdatedAt  int64
	}
	entries := make([]entry, 0, len(runs))
	for _, r := range runs {
		var latest int64
		for _, res := range r.Results {
			if t := res.CreatedAt.Unix(); t > latest {
				latest = t
			}
		}
		entries = append(entries, entry{r.ID, r.Status, r.TotalTasks, len(r.Results), latest})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID.String() < entries[j].ID.String() })
	b, _ := json.Marshal(entries)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
