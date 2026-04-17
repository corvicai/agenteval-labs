package handlers

import (
	"github.com/google/uuid"
)

// MockHub implements api.HubInterface for testing
type MockHub struct{}

func (m *MockHub) BroadcastEvent(workspaceID uuid.UUID, resource string, action string, data any) error {
	return nil
}

func (m *MockHub) BroadcastToQuestionSetAudience(questionSetID uuid.UUID, msg []byte) {}

func (m *MockHub) SendEventToQS(questionSetID uuid.UUID, eventType, correlationID string, payload any) error {
	return nil
}

func (m *MockHub) SendEventForRun(runID uuid.UUID, eventType, correlationID string, payload any) error {
	return nil
}

func (m *MockHub) SendEventToUser(userID uuid.UUID, eventType, correlationID string, payload any) error {
	return nil
}

func (m *MockHub) SendEventForAgent(agentID uuid.UUID, eventType, correlationID string, payloadForOwner, payloadForCollab any) error {
	return nil
}

func (m *MockHub) InvalidateAudienceCache(questionSetID uuid.UUID) {}

func (m *MockHub) InvalidateAgentAudienceCache(agentID uuid.UUID) {}

func (m *MockHub) InvalidateRunQSCache(runID uuid.UUID) {}
