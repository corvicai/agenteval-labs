package orchestrator

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestParseQuestionSetMaps(t *testing.T) {
	tests := []struct {
		name              string
		jsonData          string
		expectedQuestions map[string]string
		expectedAnswers   map[string]string
	}{
		{
			name: "Standard structure with explicit IDs",
			jsonData: `{
				"categories": [
					{
						"questions": [
							{
								"id": "q-1",
								"question": "What is 2+2?",
								"expected": "4"
							}
						]
					}
				]
			}`,
			expectedQuestions: map[string]string{
				"q-1": "What is 2+2?",
				"1-1": "What is 2+2?",
			},
			expectedAnswers: map[string]string{
				"q-1": "4",
				"1-1": "4",
			},
		},
		{
			name: "Missing IDs - fallback used",
			jsonData: `{
				"categories": [
					{
						"questions": [
							{
								"question": "What is 3+3?",
								"expected_answer": "6"
							}
						]
					}
				]
			}`,
			expectedQuestions: map[string]string{
				"1-1": "What is 3+3?",
			},
			expectedAnswers: map[string]string{
				"1-1": "6",
			},
		},
		{
			name: "Alternative keys (text, expected_answer)",
			jsonData: `{
				"categories": [
					{
						"questions": [
							{
								"question_id": "cust-99",
								"text": "Hello world",
								"expected_answer": "Hi"
							}
						]
					}
				]
			}`,
			expectedQuestions: map[string]string{
				"cust-99": "Hello world",
				"1-1":     "Hello world",
			},
			expectedAnswers: map[string]string{
				"cust-99": "Hi",
				"1-1":     "Hi",
			},
		},
		{
			name:     "Stringified JSON data",
			jsonData: `"{ \"categories\": [ { \"questions\": [ { \"id\": \"s-1\", \"question\": \"Stringified\" } ] } ] }"`,
			expectedQuestions: map[string]string{
				"s-1": "Stringified",
				"1-1": "Stringified",
			},
			expectedAnswers: map[string]string{
				"s-1": "",
				"1-1": "",
			},
		},
		{
			name: "Empty expected field should be preserved",
			jsonData: `{
				"categories": [
					{
						"questions": [
							{
								"id": "q-empty",
								"question": "Does it work?",
								"expected": ""
							}
						]
					}
				]
			}`,
			expectedQuestions: map[string]string{
				"q-empty": "Does it work?",
				"1-1":     "Does it work?",
			},
			expectedAnswers: map[string]string{
				"q-empty": "",
				"1-1":     "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qs, es := parseQuestionSetMaps(datatypes.JSON(tt.jsonData))
			assert.Equal(t, tt.expectedQuestions, qs)
			assert.Equal(t, tt.expectedAnswers, es)
		})
	}
}

func TestFirstNonEmptyString(t *testing.T) {
	m := map[string]any{
		"a": "",
		"b": "found",
		"c": "ignored",
	}
	assert.Equal(t, "found", firstNonEmptyString(m, "a", "b", "c"))
	assert.Equal(t, "", firstNonEmptyString(m, "x", "y"))

	m2 := map[string]any{
		"a": 123, // not a string
		"b": "real",
	}
	assert.Equal(t, "123", firstNonEmptyString(m2, "a", "b"))
}

func TestExtractTargetAgentID(t *testing.T) {
	agentID := uuid.New()
	qID := "q-123"

	tests := []struct {
		name           string
		questionID     string
		expectedID     uuid.UUID
		expectedRealID string
	}{
		{
			name:           "Valid eval prefix",
			questionID:     "eval-" + agentID.String() + "-" + qID,
			expectedID:     agentID,
			expectedRealID: qID,
		},
		{
			name:           "No eval prefix",
			questionID:     qID,
			expectedID:     uuid.Nil,
			expectedRealID: "",
		},
		{
			name:           "Invalid UUID",
			questionID:     "eval-not-a-uuid-q1",
			expectedID:     uuid.Nil,
			expectedRealID: "",
		},
		{
			name:           "Too short",
			questionID:     "eval-abc",
			expectedID:     uuid.Nil,
			expectedRealID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, realID := extractTargetAgentID(tt.questionID)
			assert.Equal(t, tt.expectedID, id)
			assert.Equal(t, tt.expectedRealID, realID)
		})
	}
}
