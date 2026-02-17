package api

import (
	"testing"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeResultEvaluationsForDisplay_PreservesExistingUserEvaluation(t *testing.T) {
	userRatingCode := 1
	userScore := 100
	result := models.RunResult{
		ID: uuid.New(),
		Evaluations: []models.Evaluation{
			{
				ID:          uuid.New(),
				RunResultID: uuid.New(),
				RaterType:   "agent",
				RaterID:     uuid.New(),
				Rating:      "wrong",
				RatingCode:  intPtr(4),
				Score:       intPtr(0),
				CreatedAt:   time.Now().Add(-time.Minute),
			},
			{
				ID:          uuid.New(),
				RunResultID: uuid.New(),
				RaterType:   "user",
				RaterID:     uuid.New(),
				Rating:      "like",
				RatingCode:  &userRatingCode,
				Score:       &userScore,
				CreatedAt:   time.Now(),
			},
		},
	}

	normalizeResultEvaluationsForDisplay(&result)

	require.Len(t, result.Evaluations, 2)
	userCount := 0
	for _, ev := range result.Evaluations {
		if ev.RaterType == "user" {
			userCount++
		}
	}
	assert.Equal(t, 1, userCount)
}

func TestNormalizeResultEvaluationsForDisplay_AddsSyntheticUserFromLatestAgent(t *testing.T) {
	olderAgentTime := time.Now().Add(-2 * time.Minute)
	newerAgentTime := time.Now().Add(-time.Minute)

	oldRatingCode := 2
	oldScore := 60
	newRatingCode := 4
	newScore := 0

	result := models.RunResult{
		ID: uuid.New(),
		Evaluations: []models.Evaluation{
			{
				ID:          uuid.New(),
				RunResultID: uuid.New(),
				RaterType:   "agent",
				RaterID:     uuid.New(),
				Rating:      "valid",
				RatingCode:  &oldRatingCode,
				Score:       &oldScore,
				CreatedAt:   olderAgentTime,
			},
			{
				ID:          uuid.New(),
				RunResultID: uuid.New(),
				RaterType:   "agent",
				RaterID:     uuid.New(),
				Rating:      "wrong",
				RatingCode:  &newRatingCode,
				Score:       &newScore,
				CreatedAt:   newerAgentTime,
			},
		},
	}

	normalizeResultEvaluationsForDisplay(&result)

	require.Len(t, result.Evaluations, 3)

	var userEvals []models.Evaluation
	for _, ev := range result.Evaluations {
		if ev.RaterType == "user" {
			userEvals = append(userEvals, ev)
		}
	}
	require.Len(t, userEvals, 1)
	assert.Equal(t, "dislike", userEvals[0].Rating)
	if assert.NotNil(t, userEvals[0].RatingCode) {
		assert.Equal(t, 3, *userEvals[0].RatingCode)
	}
	if assert.NotNil(t, userEvals[0].Score) {
		assert.Equal(t, 0, *userEvals[0].Score)
	}
	assert.Equal(t, uuid.Nil, userEvals[0].RaterID)
}

func TestNormalizeResultEvaluationsForDisplay_NoEvaluations(t *testing.T) {
	result := models.RunResult{
		ID:          uuid.New(),
		Evaluations: []models.Evaluation{},
	}

	normalizeResultEvaluationsForDisplay(&result)
	require.Empty(t, result.Evaluations)
}

func intPtr(v int) *int {
	return &v
}
