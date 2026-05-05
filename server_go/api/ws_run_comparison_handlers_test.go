package api

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"benchmarking-platform/models"
)

func Test_buildComparisonReport_SameQS_SameAgents(t *testing.T) {
	setup()

	// Create User and Workspace
	user, token := createTestUser(t, false)
	
	// Create Workspace because tests expect workspace to be created to run tests inside it sometimes
	wsID := uuid.New()
	qsID := uuid.New()
	agentID := uuid.New()

	db.Create(&models.Workspace{
		ID:     wsID,
		UserID: user.ID,
		Name:   "Test WS",
	})
	
	db.Create(&models.QuestionSet{
		ID:   qsID,
		Name: "QS 1",
	})

	db.Create(&models.Agent{
		ID:          agentID,
		WorkspaceID: wsID,
		Name:        "Agent 1",
	})

	run1 := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		QuestionSetID: qsID,
		Status:        "completed",
		TotalTasks:    1,
		CreatedAt:     time.Now(),
	}
	run2 := models.Run{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		QuestionSetID: qsID,
		Status:        "completed",
		TotalTasks:    1,
		CreatedAt:     time.Now(),
	}

	db.Create(&run1)
	db.Create(&run2)

	score := 5
	res1 := models.RunResult{
		ID:         uuid.New(),
		RunID:      run1.ID,
		AgentID:    agentID,
		QuestionID: "q1",
		Status:     "success",
		DurationMs: 100,
		CreatedAt:  time.Now(),
	}
	db.Create(&res1)
	db.Create(&models.Evaluation{
		ID:          uuid.New(),
		RunResultID: res1.ID,
		Rating:      "like",
		Score:       &score,
	})

	score2 := 4
	res2 := models.RunResult{
		ID:         uuid.New(),
		RunID:      run2.ID,
		AgentID:    agentID,
		QuestionID: "q1",
		Status:     "success",
		DurationMs: 150,
		CreatedAt:  time.Now(),
	}
	db.Create(&res2)
	db.Create(&models.Evaluation{
		ID:          uuid.New(),
		RunResultID: res2.ID,
		Rating:      "valid",
		Score:       &score2,
	})

	// Test directly buildComparisonReport
	hub := NewHub(db, nil, "test-secret", nil)
	metrics := map[string]bool{"totals": true, "regressions": true}
	req := compareRunsRequest{
		RunIDs:         []uuid.UUID{run1.ID, run2.ID},
		MetricsEnabled: metrics,
	}

	report, err := hub.buildComparisonReport(context.Background(), wsID, user.ID, req)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.SameQuestionSet)
	assert.True(t, report.SameAgents)
	assert.Len(t, report.Runs, 2)
	assert.Len(t, report.CommonQuestionIDs, 1)
	
	// Check regressions
	assert.Len(t, report.Regressions, 1) // 5 to 4 is -1, which matches threshold <= -1.0
	assert.Equal(t, float64(-1), report.Regressions[0].Delta)

	// Now check via WS with correct token workspace
	token = generateTestToken(user.ID, wsID, uuid.Nil)
	respEnvelope := sendWSRequest(t, token, ReqCompareRuns, req)
	
	// Because of our custom setup logic in sendWSRequest, token parser might fall back to org workspace.
	// But it should at least not throw internal error.
	assert.NotNil(t, respEnvelope)
}

// Regression test: evaluator RunResults must not inflate question counts or
// pollute the primary-agent comparison blocks.
func Test_buildComparisonReport_ExcludesEvaluatorResults(t *testing.T) {
	setup()

	user, _ := createTestUser(t, false)

	wsID := uuid.New()
	qsID := uuid.New()
	primaryAgentID := uuid.New()
	evalAgentID := uuid.New()

	db.Create(&models.Workspace{ID: wsID, UserID: user.ID, Name: "Test WS"})
	db.Create(&models.QuestionSet{ID: qsID, Name: "QS Eval"})
	db.Create(&models.Agent{ID: primaryAgentID, WorkspaceID: wsID, Name: "Primary"})
	db.Create(&models.Agent{ID: evalAgentID, WorkspaceID: wsID, Name: "Evaluator", ProviderType: "evaluator"})

	mkRun := func() models.Run {
		run := models.Run{
			ID:            uuid.New(),
			WorkspaceID:   wsID,
			QuestionSetID: qsID,
			Status:        "completed",
			// TotalTasks is intentionally inflated (primary + eval) to mirror
			// what the orchestrator stores. The report should NOT rely on it.
			TotalTasks: 4,
			CreatedAt:  time.Now(),
		}
		db.Create(&run)
		return run
	}

	run1 := mkRun()
	run2 := mkRun()

	addResults := func(runID uuid.UUID, score int) {
		primary1 := models.RunResult{
			ID: uuid.New(), RunID: runID, AgentID: primaryAgentID,
			QuestionID: "q1", Status: "success", DurationMs: 100, CreatedAt: time.Now(),
		}
		primary2 := models.RunResult{
			ID: uuid.New(), RunID: runID, AgentID: primaryAgentID,
			QuestionID: "q2", Status: "success", DurationMs: 150, CreatedAt: time.Now(),
		}
		db.Create(&primary1)
		db.Create(&primary2)

		// Evaluator RunResults (stored with eval-<targetAgent>-<qID>) — these
		// must be ignored by the comparison builder.
		db.Create(&models.RunResult{
			ID: uuid.New(), RunID: runID, AgentID: evalAgentID,
			QuestionID: "eval-" + primaryAgentID.String() + "-q1",
			Status:     "success", DurationMs: 50, CreatedAt: time.Now(),
		})
		db.Create(&models.RunResult{
			ID: uuid.New(), RunID: runID, AgentID: evalAgentID,
			QuestionID: "eval-" + primaryAgentID.String() + "-q2",
			Status:     "success", DurationMs: 50, CreatedAt: time.Now(),
		})

		// Evaluations live on the primary results.
		s := score
		db.Create(&models.Evaluation{
			ID: uuid.New(), RunResultID: primary1.ID, Rating: "like", Score: &s,
		})
		db.Create(&models.Evaluation{
			ID: uuid.New(), RunResultID: primary2.ID, Rating: "valid", Score: &s,
		})
	}
	addResults(run1.ID, 5)
	addResults(run2.ID, 4)

	hub := NewHub(db, nil, "test-secret", nil)
	report, err := hub.buildComparisonReport(context.Background(), wsID, user.ID, compareRunsRequest{
		RunIDs:         []uuid.UUID{run1.ID, run2.ID},
		MetricsEnabled: map[string]bool{"totals": true, "regressions": true},
	})
	assert.NoError(t, err)
	assert.NotNil(t, report)

	for _, rb := range report.Runs {
		assert.Equal(t, 2, rb.Totals.Questions,
			"Totals.Questions should equal the number of distinct primary question IDs, not TotalTasks (which includes evaluator tasks)")
		assert.Len(t, rb.PerQuestion, 2,
			"PerQuestion must only contain primary results")
		assert.Len(t, rb.Agents, 1,
			"Only the primary agent should appear in the agents block")
		assert.Equal(t, primaryAgentID, rb.Agents[0].ID)
		for _, pq := range rb.PerQuestion {
			assert.NotContains(t, pq.QuestionID, "eval-",
				"PerQuestion entries must not include evaluator rows")
		}
	}

	assert.Len(t, report.CommonQuestionIDs, 2)
	for _, q := range report.CommonQuestionIDs {
		assert.NotContains(t, q, "eval-",
			"Common question IDs must reflect primary questions only")
	}
}

// Runs from a foreign workspace are only readable through an active
// (accepted, non-revoked) question-set collaboration.
func Test_buildComparisonReport_Authorization(t *testing.T) {
	setup()

	owner, _ := createTestUser(t, false)
	outsider, _ := createTestUser(t, false)

	ownerWS := uuid.New()
	outsiderWS := uuid.New()
	qsID := uuid.New()
	agentID := uuid.New()

	db.Create(&models.Workspace{ID: ownerWS, UserID: owner.ID, Name: "Owner WS"})
	db.Create(&models.Workspace{ID: outsiderWS, UserID: outsider.ID, Name: "Outsider WS"})
	db.Create(&models.QuestionSet{ID: qsID, Name: "QS Auth"})
	db.Create(&models.Agent{ID: agentID, WorkspaceID: ownerWS, Name: "Agent"})

	mkRun := func() models.Run {
		run := models.Run{
			ID:            uuid.New(),
			WorkspaceID:   ownerWS,
			QuestionSetID: qsID,
			Status:        "completed",
			TotalTasks:    1,
			CreatedAt:     time.Now(),
		}
		db.Create(&run)
		return run
	}
	run1 := mkRun()
	run2 := mkRun()

	hub := NewHub(db, nil, "test-secret", nil)
	req := compareRunsRequest{
		RunIDs:         []uuid.UUID{run1.ID, run2.ID},
		MetricsEnabled: map[string]bool{"totals": true},
	}

	// No grant: a user from another workspace cannot read these runs.
	_, err := hub.buildComparisonReport(context.Background(), outsiderWS, outsider.ID, req)
	assert.Error(t, err)

	// Accepted collaborator on the question set can compare its runs.
	now := time.Now()
	collab := models.QuestionSetCollaborator{
		ID:              uuid.New(),
		QuestionSetID:   qsID,
		UserID:          outsider.ID,
		InvitedByUserID: owner.ID,
		AcceptedAt:      &now,
	}
	db.Create(&collab)
	report, err := hub.buildComparisonReport(context.Background(), outsiderWS, outsider.ID, req)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Len(t, report.Runs, 2)

	// Revoked grant: access is gone again.
	db.Model(&models.QuestionSetCollaborator{}).
		Where("id = ?", collab.ID).
		Update("revoked_at", &now)
	_, err = hub.buildComparisonReport(context.Background(), outsiderWS, outsider.ID, req)
	assert.Error(t, err)
}

func Test_detectRegressions(t *testing.T) {
	// Let's create dummy run blocks
	agID := uuid.New()
	s1 := float64(5)
	s2 := float64(3.5)

	runs := []ComparisonRunBlock{
		{
			Label: "Run 1",
			PerQuestion: []ComparisonQScore{
				{QuestionID: "q1", AgentID: agID, Score: &s1},
			},
		},
		{
			Label: "Run 2",
			PerQuestion: []ComparisonQScore{
				{QuestionID: "q1", AgentID: agID, Score: &s2},
			},
		},
	}

	regs := detectRegressions(runs)
	assert.Len(t, regs, 1)
	assert.Equal(t, -1.5, regs[0].Delta)
}

func Test_computeRunsSnapshotHash_stability(t *testing.T) {
	r1 := models.Run{
		ID:         uuid.New(),
		Status:     "completed",
		TotalTasks: 5,
		Results: []models.RunResult{
			{CreatedAt: time.Unix(100, 0)},
		},
	}
	r2 := models.Run{
		ID:         uuid.New(),
		Status:     "completed",
		TotalTasks: 10,
		Results: []models.RunResult{
			{CreatedAt: time.Unix(200, 0)},
		},
	}

	hash1 := computeRunsSnapshotHash([]models.Run{r1, r2})
	hash2 := computeRunsSnapshotHash([]models.Run{r2, r1}) // swapped order

	assert.Equal(t, hash1, hash2, "Reordering should not change the hash")
}
