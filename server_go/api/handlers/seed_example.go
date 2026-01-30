package handlers

import (
	"encoding/json"
	"log"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

func seedExampleSetupIfFirstWorkspace(db *gorm.DB, userID uuid.UUID, workspace models.Workspace, client models.Client) {
	if db == nil || userID == uuid.Nil || workspace.ID == uuid.Nil || client.ID == uuid.Nil {
		return
	}

	var workspaceCount int64
	if err := db.Model(&models.Workspace{}).Where("user_id = ?", userID).Count(&workspaceCount).Error; err != nil {
		log.Printf("[seed] Failed to count workspaces for user %s: %v", userID, err)
		return
	}
	if workspaceCount != 1 {
		return
	}

	var agentCount int64
	if err := db.Model(&models.Agent{}).Where("workspace_id = ?", workspace.ID).Count(&agentCount).Error; err != nil {
		log.Printf("[seed] Failed to count agents for workspace %s: %v", workspace.ID, err)
		return
	}
	var questionSetCount int64
	if err := db.Model(&models.QuestionSet{}).Where("client_id = ?", client.ID).Count(&questionSetCount).Error; err != nil {
		log.Printf("[seed] Failed to count question sets for client %s: %v", client.ID, err)
		return
	}
	if agentCount > 0 || questionSetCount > 0 {
		return
	}

	agentConfig := map[string]any{
		"mode":     "http",
		"endpoint": "",
		"token":    "",
	}
	agentConfigBytes, err := json.Marshal(agentConfig)
	if err != nil {
		log.Printf("[seed] Failed to marshal example agent config: %v", err)
		return
	}

	agent := models.Agent{
		ID:           uuid.New(),
		WorkspaceID:  workspace.ID,
		Name:         "Example Agent (configure me)",
		ProviderType: "mcp",
		Config:       models.EncryptedJSON(agentConfigBytes),
		Enabled:      true,
		Position:     0,
	}
	if err := db.Create(&agent).Error; err != nil {
		log.Printf("[seed] Failed to create example agent: %v", err)
		return
	}

	questionData := exampleQuestionSetData()
	questionDataBytes, err := json.Marshal(questionData)
	if err != nil {
		log.Printf("[seed] Failed to marshal example question set: %v", err)
		return
	}

	questionSet := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     "RAG Example Questions",
		Version:  "1.0",
		Data:     datatypes.JSON(questionDataBytes),
	}
	if err := db.Create(&questionSet).Error; err != nil {
		log.Printf("[seed] Failed to create example question set: %v", err)
	}
}

func exampleQuestionSetData() QuestionData {
	return QuestionData{
		Categories: []Category{
			{
				Name: "RAG-Oriented Questions (Knowledge Retrieval)",
				Questions: []Question{
					{ID: "RAG-KR-01", Question: "According to the provided documentation, how does the authentication flow work?"},
					{ID: "RAG-KR-02", Question: "What does the policy say about data retention?"},
					{ID: "RAG-KR-03", Question: "Based on the uploaded file, what are the main conclusions?"},
					{ID: "RAG-KR-04", Question: "Which section of the document mentions rate limiting?"},
					{ID: "RAG-KR-05", Question: "Using only the given context, explain how feature X works."},
				},
			},
			{
				Name: "RAG Consistency and Hallucination Checks",
				Questions: []Question{
					{ID: "RAG-CH-01", Question: "Is this information explicitly stated in the provided context?"},
					{ID: "RAG-CH-02", Question: "If the answer is not in the documents, say 'I don't know'."},
					{ID: "RAG-CH-03", Question: "Can you find a source in the context that supports this claim?"},
					{ID: "RAG-CH-04", Question: "What assumptions are you making based on the given text?"},
					{ID: "RAG-CH-05", Question: "Does the context contradict this answer?"},
				},
			},
			{
				Name: "Multi-Document or Chunked Context",
				Questions: []Question{
					{ID: "RAG-MD-01", Question: "Compare the approaches described in document A and document B."},
					{ID: "RAG-MD-02", Question: "Which document provides the most detailed explanation of this concept?"},
					{ID: "RAG-MD-03", Question: "Summarize the differences between the two specifications."},
					{ID: "RAG-MD-04", Question: "Are there conflicting statements across the sources?"},
					{ID: "RAG-MD-05", Question: "Merge the information from all documents into a single explanation."},
				},
			},
		},
	}
}
