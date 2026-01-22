package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

type EvaluationHandler struct {
	DB *gorm.DB
}

func NewEvaluationHandler(db *gorm.DB) *EvaluationHandler {
	return &EvaluationHandler{DB: db}
}

// CreateEvaluationRequest represents the request body
type CreateEvaluationRequest struct {
	RunResultID string `json:"run_result_id" validate:"required"`
	Rating      string `json:"rating"` // legacy
	RatingCode  *int   `json:"rating_code"`
	Score       *int   `json:"score"`
	Comments    string `json:"comments"`
}

// Create a new evaluation (human rating)
func (h *EvaluationHandler) Create(c echo.Context) error {
	// TODO: Extract user ID from JWT/session
	userID := uuid.Nil // Placeholder

	var req CreateEvaluationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	rrID, err := uuid.Parse(req.RunResultID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_result_id"})
	}

	// Logic for RatingCode and Rating mapping
	var ratingCode int
	var rating string

	if req.RatingCode != nil {
		ratingCode = *req.RatingCode
		// Map back to rating string for compatibility
		switch ratingCode {
		case 1:
			rating = "like"
		case 2:
			rating = "valid"
		case 3:
			rating = "dislike"
		case 4:
			rating = "wrong"
		default:
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid rating_code"})
		}
	} else if req.Rating != "" {
		rating = req.Rating
		switch rating {
		case "like":
			ratingCode = 1
		case "valid":
			ratingCode = 2
		case "dislike":
			ratingCode = 3
		case "wrong":
			ratingCode = 4
		default:
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid rating"})
		}
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "rating or rating_code is required"})
	}

	score := req.Score
	if score == nil {
		defaultScore := 0
		switch ratingCode {
		case 1:
			defaultScore = 100
		case 2:
			defaultScore = 75
		case 3:
			defaultScore = 25
		case 4:
			defaultScore = 0
		}
		score = &defaultScore
	}

	// Verify run result exists
	var runResult models.RunResult
	if err := h.DB.First(&runResult, "id = ?", rrID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "run result not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	eval := models.Evaluation{
		ID:          uuid.New(),
		RunResultID: rrID,
		RaterType:   "user",
		RaterID:     userID,
		Rating:      rating,
		RatingCode:  &ratingCode,
		Score:       score,
		Comments:    req.Comments,
	}

	// Remove any existing user evaluation for this result to ensure only the latest is kept
	if err := h.DB.Delete(&models.Evaluation{}, "run_result_id = ? AND rater_type = ?", rrID, "user").Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if err := h.DB.Create(&eval).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, eval)
}

// List evaluations for a run result
func (h *EvaluationHandler) List(c echo.Context) error {
	runResultID := c.Param("run_result_id")
	rrID, err := uuid.Parse(runResultID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_result_id"})
	}

	var evals []models.Evaluation
	if err := h.DB.Where("run_result_id = ?", rrID).Order("created_at DESC").Find(&evals).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, evals)
}

// Delete an evaluation
func (h *EvaluationHandler) Delete(c echo.Context) error {
	evalID := c.Param("id")
	id, err := uuid.Parse(evalID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if err := h.DB.Delete(&models.Evaluation{}, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
