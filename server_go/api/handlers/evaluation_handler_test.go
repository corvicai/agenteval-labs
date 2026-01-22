package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

func setupEvaluationTestDB(t *testing.T) (*gorm.DB, uuid.UUID) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Workspace{},
		&models.Client{},
		&models.Agent{},
		&models.QuestionSet{},
		&models.Run{},
		&models.RunResult{},
		&models.Evaluation{},
	)
	require.NoError(t, err)

	// Create chain: user -> workspace -> client -> question set -> run -> result
	user := models.User{ID: uuid.New(), Name: "Test", Email: "test@test.com"}
	db.Create(&user)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "Test WS"}
	db.Create(&workspace)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "Test Client"}
	db.Create(&client)

	agent := models.Agent{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "Agent", ProviderType: "mcp"}
	db.Create(&agent)

	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: []byte(`{}`)}
	db.Create(&qs)

	run := models.Run{ID: uuid.New(), WorkspaceID: workspace.ID, QuestionSetID: qs.ID, Status: "completed"}
	db.Create(&run)

	result := models.RunResult{
		ID:         uuid.New(),
		RunID:      run.ID,
		AgentID:    agent.ID,
		QuestionID: "q-1",
		Status:     "success",
		Answer:     "Test answer",
		DurationMs: 100,
	}
	db.Create(&result)

	return db, result.ID
}

func TestEvaluationHandler_Create(t *testing.T) {
	db, runResultID := setupEvaluationTestDB(t)
	handler := NewEvaluationHandler(db)

	e := echo.New()

	t.Run("creates evaluation with valid rating_code", func(t *testing.T) {
		ratingCode := 1
		score := 95
		reqBody := CreateEvaluationRequest{
			RunResultID: runResultID.String(),
			RatingCode:  &ratingCode,
			Score:       &score,
			Comments:    "Great answer!",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var eval models.Evaluation
		json.Unmarshal(rec.Body.Bytes(), &eval)
		assert.Equal(t, "like", eval.Rating)
		assert.Equal(t, 1, *eval.RatingCode)
		assert.Equal(t, 95, *eval.Score)
		assert.Equal(t, "Great answer!", eval.Comments)
		assert.Equal(t, "user", eval.RaterType)
	})

	t.Run("creates evaluation with mapping from legacy rating string", func(t *testing.T) {
		reqBody := CreateEvaluationRequest{
			RunResultID: runResultID.String(),
			Rating:      "dislike",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var eval models.Evaluation
		json.Unmarshal(rec.Body.Bytes(), &eval)
		assert.Equal(t, "dislike", eval.Rating)
		assert.Equal(t, 3, *eval.RatingCode)
	})

	t.Run("rejects invalid rating_code", func(t *testing.T) {
		ratingCode := 99
		reqBody := CreateEvaluationRequest{
			RunResultID: runResultID.String(),
			RatingCode:  &ratingCode,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("allows all valid rating types", func(t *testing.T) {
		validRatings := []string{"like", "dislike", "valid", "wrong"}

		for _, rating := range validRatings {
			reqBody := CreateEvaluationRequest{
				RunResultID: runResultID.String(),
				Rating:      rating,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Create(c)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusCreated, rec.Code, "Rating '%s' should be valid", rating)
		}
	})

	t.Run("fails for non-existent run result", func(t *testing.T) {
		reqBody := CreateEvaluationRequest{
			RunResultID: uuid.New().String(), // Non-existent
			Rating:      "like",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Create(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestEvaluationHandler_List(t *testing.T) {
	db, runResultID := setupEvaluationTestDB(t)
	handler := NewEvaluationHandler(db)

	// Create multiple evaluations
	for i := 0; i < 3; i++ {
		eval := models.Evaluation{
			ID:          uuid.New(),
			RunResultID: runResultID,
			RaterType:   "user",
			Rating:      "like",
		}
		db.Create(&eval)
	}

	e := echo.New()

	t.Run("lists evaluations for run result", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("run_result_id")
		c.SetParamValues(runResultID.String())

		err := handler.List(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var evals []models.Evaluation
		json.Unmarshal(rec.Body.Bytes(), &evals)
		assert.Len(t, evals, 3)
	})
}

func TestEvaluationHandler_Delete(t *testing.T) {
	db, runResultID := setupEvaluationTestDB(t)
	handler := NewEvaluationHandler(db)

	eval := models.Evaluation{
		ID:          uuid.New(),
		RunResultID: runResultID,
		RaterType:   "user",
		Rating:      "dislike",
	}
	db.Create(&eval)

	e := echo.New()

	t.Run("deletes evaluation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(eval.ID.String())

		err := handler.Delete(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)

		// Verify deleted
		var count int64
		db.Model(&models.Evaluation{}).Where("id = ?", eval.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})
}

// Test that evaluations are linked to specific run results (not questions abstractly)
func TestEvaluation_LinkedToRunResult(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Workspace{}, &models.Client{},
		&models.Agent{}, &models.QuestionSet{}, &models.Run{}, &models.RunResult{}, &models.Evaluation{}))

	// Setup
	user := models.User{ID: uuid.New(), Name: "Test", Email: "test@test.com"}
	db.Create(&user)
	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "WS"}
	db.Create(&workspace)
	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "C"}
	db.Create(&client)
	agent := models.Agent{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "A", ProviderType: "mcp"}
	db.Create(&agent)
	qs := models.QuestionSet{ID: uuid.New(), ClientID: client.ID, Name: "QS", Data: []byte(`{}`)}
	db.Create(&qs)

	// Run 1
	run1 := models.Run{ID: uuid.New(), WorkspaceID: workspace.ID, QuestionSetID: qs.ID, Status: "completed"}
	db.Create(&run1)
	result1 := models.RunResult{ID: uuid.New(), RunID: run1.ID, AgentID: agent.ID, QuestionID: "q-1", Answer: "Answer v1"}
	db.Create(&result1)

	// Run 2 (rerun)
	run2 := models.Run{ID: uuid.New(), WorkspaceID: workspace.ID, QuestionSetID: qs.ID, Status: "completed"}
	db.Create(&run2)
	result2 := models.RunResult{ID: uuid.New(), RunID: run2.ID, AgentID: agent.ID, QuestionID: "q-1", Answer: "Answer v2"}
	db.Create(&result2)

	// Rate result1 as "like"
	eval1 := models.Evaluation{ID: uuid.New(), RunResultID: result1.ID, RaterType: "user", Rating: "like"}
	db.Create(&eval1)

	// Rate result2 as "dislike"
	eval2 := models.Evaluation{ID: uuid.New(), RunResultID: result2.ID, RaterType: "user", Rating: "dislike"}
	db.Create(&eval2)

	t.Run("evaluations are tied to specific run results", func(t *testing.T) {
		var evalForResult1 models.Evaluation
		db.Where("run_result_id = ?", result1.ID).First(&evalForResult1)
		assert.Equal(t, "like", evalForResult1.Rating)

		var evalForResult2 models.Evaluation
		db.Where("run_result_id = ?", result2.ID).First(&evalForResult2)
		assert.Equal(t, "dislike", evalForResult2.Rating)

		// Different evaluations for same question but different runs
		assert.NotEqual(t, evalForResult1.Rating, evalForResult2.Rating)
	})
}
