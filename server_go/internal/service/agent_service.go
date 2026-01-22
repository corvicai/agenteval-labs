package service

import (
	"encoding/json"
	"strings"

	"benchmarking-platform/models"

	"github.com/google/uuid"
)

type AgentService struct {
	db any // *gorm.DB
}

func (s *AgentService) GetSpyPayload(agent *models.Agent, question string) (map[string]any, error) {
	config := make(map[string]any)
	json.Unmarshal(agent.Config, &config)

	// Redact sensitive fields
	sensitiveKeys := []string{"token", "api_key", "secret", "password"}
	redactedConfig := make(map[string]any)
	for k, v := range config {
		isSensitive := false
		for _, sk := range sensitiveKeys {
			if strings.Contains(strings.ToLower(k), sk) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			if val, ok := v.(string); ok && len(val) > 4 {
				redactedConfig[k] = val[:2] + "****" + val[len(val)-2:]
			} else {
				redactedConfig[k] = "****"
			}
		} else {
			redactedConfig[k] = v
		}
	}

	payload := map[string]any{
		"provider_type": agent.ProviderType,
		"config":        redactedConfig,
		"payload": map[string]any{
			"question": question,
		},
	}

	return payload, nil
}

func (s *AgentService) ReorderAgents(workspaceID uuid.UUID, agentIDs []uuid.UUID) error {
	// Logic to update position field in database for each agent ID
	return nil
}
