package orchestrator

import (
	"sort"

	"benchmarking-platform/models"
)

func resultLogicalKey(agentID string, questionID string) string {
	return agentID + "::" + questionID
}

func isRunResultNewer(candidate models.RunResult, current models.RunResult) bool {
	if candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.ID.String() > current.ID.String()
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func isRunResultLiteNewer(candidate models.RunResultLite, current models.RunResultLite) bool {
	if candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.ID.String() > current.ID.String()
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func CollapseRunResultsToLatest(results []models.RunResult) []models.RunResult {
	if len(results) <= 1 {
		return results
	}

	latestByKey := make(map[string]models.RunResult, len(results))
	for _, result := range results {
		key := resultLogicalKey(result.AgentID.String(), result.QuestionID)
		current, exists := latestByKey[key]
		if !exists || isRunResultNewer(result, current) {
			latestByKey[key] = result
		}
	}

	collapsed := make([]models.RunResult, 0, len(latestByKey))
	for _, result := range latestByKey {
		collapsed = append(collapsed, result)
	}

	sort.SliceStable(collapsed, func(i, j int) bool {
		if collapsed[i].CreatedAt.Equal(collapsed[j].CreatedAt) {
			return collapsed[i].ID.String() < collapsed[j].ID.String()
		}
		return collapsed[i].CreatedAt.Before(collapsed[j].CreatedAt)
	})

	return collapsed
}

func CollapseRunResultLitesToLatest(results []models.RunResultLite) []models.RunResultLite {
	if len(results) <= 1 {
		return results
	}

	latestByKey := make(map[string]models.RunResultLite, len(results))
	for _, result := range results {
		key := resultLogicalKey(result.AgentID.String(), result.QuestionID)
		current, exists := latestByKey[key]
		if !exists || isRunResultLiteNewer(result, current) {
			latestByKey[key] = result
		}
	}

	collapsed := make([]models.RunResultLite, 0, len(latestByKey))
	for _, result := range latestByKey {
		collapsed = append(collapsed, result)
	}

	sort.SliceStable(collapsed, func(i, j int) bool {
		if collapsed[i].CreatedAt.Equal(collapsed[j].CreatedAt) {
			return collapsed[i].ID.String() < collapsed[j].ID.String()
		}
		return collapsed[i].CreatedAt.Before(collapsed[j].CreatedAt)
	})

	return collapsed
}
