package handlers

import (
	"github.com/google/uuid"
)

// MockHub implements api.HubInterface for testing
type MockHub struct{}

func (m *MockHub) BroadcastEvent(workspaceID uuid.UUID, resource string, action string, data any) error {
	return nil
}
