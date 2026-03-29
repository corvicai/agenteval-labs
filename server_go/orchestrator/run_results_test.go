package orchestrator

import (
	"testing"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollapseRunResultsToLatest(t *testing.T) {
	agentID := uuid.New()
	now := time.Now().UTC()

	older := models.RunResult{
		ID:         uuid.New(),
		AgentID:    agentID,
		QuestionID: "q-1",
		Answer:     "old answer",
		CreatedAt:  now.Add(-time.Minute),
	}
	newer := models.RunResult{
		ID:         uuid.New(),
		AgentID:    agentID,
		QuestionID: "q-1",
		Answer:     "new answer",
		CreatedAt:  now,
	}
	other := models.RunResult{
		ID:         uuid.New(),
		AgentID:    agentID,
		QuestionID: "q-2",
		Answer:     "other answer",
		CreatedAt:  now,
	}

	collapsed := CollapseRunResultsToLatest([]models.RunResult{older, other, newer})
	require.Len(t, collapsed, 2)

	byQuestion := map[string]models.RunResult{}
	for _, item := range collapsed {
		byQuestion[item.QuestionID] = item
	}

	assert.Equal(t, newer.ID, byQuestion["q-1"].ID)
	assert.Equal(t, "new answer", byQuestion["q-1"].Answer)
	assert.Equal(t, other.ID, byQuestion["q-2"].ID)
}

func TestCollapseRunResultLitesToLatest(t *testing.T) {
	agentID := uuid.New()
	now := time.Now().UTC()

	older := models.RunResultLite{
		ID:         uuid.New(),
		AgentID:    agentID,
		QuestionID: "q-1",
		Status:     "error",
		CreatedAt:  now.Add(-time.Minute),
	}
	newer := models.RunResultLite{
		ID:         uuid.New(),
		AgentID:    agentID,
		QuestionID: "q-1",
		Status:     "success",
		CreatedAt:  now,
	}

	collapsed := CollapseRunResultLitesToLatest([]models.RunResultLite{older, newer})
	require.Len(t, collapsed, 1)
	assert.Equal(t, newer.ID, collapsed[0].ID)
	assert.Equal(t, "success", collapsed[0].Status)
}
