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

func TestExtractEvaluatorScore(t *testing.T) {
	tests := []struct {
		name      string
		answer    string
		wantScore int
		wantOK    bool
	}{
		{
			name:      "score at end",
			answer:    "Good response.\n7/10",
			wantScore: 7,
			wantOK:    true,
		},
		{
			name:      "multiple scores keeps last valid",
			answer:    "First pass 3/10, after review: 8/10",
			wantScore: 8,
			wantOK:    true,
		},
		{
			name:      "score with spaces",
			answer:    "Final score: 10 / 10",
			wantScore: 10,
			wantOK:    true,
		},
		{
			name:      "markdown wrapper",
			answer:    "Final score **9/10**",
			wantScore: 9,
			wantOK:    true,
		},
		{
			name:      "invalid denominator",
			answer:    "5/5",
			wantScore: 0,
			wantOK:    false,
		},
		{
			name:      "invalid numerator",
			answer:    "11/10",
			wantScore: 0,
			wantOK:    false,
		},
		{
			name:      "no score",
			answer:    "No explicit score",
			wantScore: 0,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotOK := extractEvaluatorScore(tt.answer)
			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantScore, gotScore)
		})
	}
}

func TestParseOpenAIResponses(t *testing.T) {
	t.Run("reasoning item before message is handled", func(t *testing.T) {
		decoded := map[string]any{
			"output": []any{
				map[string]any{
					"type":    "reasoning",
					"id":      "rs_123",
					"summary": []any{},
				},
				map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": "final answer"},
					},
				},
			},
		}

		got, err := parseOpenAIResponses(decoded)
		assert.NoError(t, err)
		assert.Equal(t, "final answer", got)
	})

	t.Run("top level output_text is honored first", func(t *testing.T) {
		decoded := map[string]any{
			"output_text": "shortcut answer",
			"output": []any{
				map[string]any{
					"type":    "message",
					"content": []any{map[string]any{"type": "output_text", "text": "ignored"}},
				},
			},
		}

		got, err := parseOpenAIResponses(decoded)
		assert.NoError(t, err)
		assert.Equal(t, "shortcut answer", got)
	})

	t.Run("multiple text blocks are concatenated", func(t *testing.T) {
		decoded := map[string]any{
			"output": []any{
				map[string]any{
					"type": "message",
					"content": []any{
						map[string]any{"type": "output_text", "text": "part one"},
						map[string]any{"type": "output_text", "text": "part two"},
					},
				},
			},
		}

		got, err := parseOpenAIResponses(decoded)
		assert.NoError(t, err)
		assert.Equal(t, "part one\npart two", got)
	})

	t.Run("legacy untyped content with text field is accepted", func(t *testing.T) {
		decoded := map[string]any{
			"output": []any{
				map[string]any{
					"content": []any{
						map[string]any{"text": "legacy shape"},
					},
				},
			},
		}

		got, err := parseOpenAIResponses(decoded)
		assert.NoError(t, err)
		assert.Equal(t, "legacy shape", got)
	})

	t.Run("only reasoning item returns diagnostic error", func(t *testing.T) {
		decoded := map[string]any{
			"output": []any{
				map[string]any{"type": "reasoning", "id": "rs_1"},
				map[string]any{"type": "tool_use", "id": "t_1"},
			},
		}

		_, err := parseOpenAIResponses(decoded)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reasoning")
		assert.Contains(t, err.Error(), "tool_use")
	})

	t.Run("empty output array yields diagnostic", func(t *testing.T) {
		_, err := parseOpenAIResponses(map[string]any{"output": []any{}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no output array")
	})

	t.Run("empty output_text falls through to output parsing", func(t *testing.T) {
		decoded := map[string]any{
			"output_text": "   ",
			"output": []any{
				map[string]any{
					"type":    "message",
					"content": []any{map[string]any{"type": "output_text", "text": "real answer"}},
				},
			},
		}

		got, err := parseOpenAIResponses(decoded)
		assert.NoError(t, err)
		assert.Equal(t, "real answer", got)
	})
}

func TestMapEvaluatorScore(t *testing.T) {
	tests := []struct {
		score10        int
		expectedRating string
		expectedCode   int
		expectedScore  int
	}{
		{score10: 10, expectedRating: "like", expectedCode: 1, expectedScore: 100},
		{score10: 8, expectedRating: "like", expectedCode: 1, expectedScore: 80},
		{score10: 7, expectedRating: "valid", expectedCode: 2, expectedScore: 70},
		{score10: 6, expectedRating: "valid", expectedCode: 2, expectedScore: 60},
		{score10: 5, expectedRating: "dislike", expectedCode: 3, expectedScore: 50},
		{score10: 3, expectedRating: "dislike", expectedCode: 3, expectedScore: 30},
		{score10: 2, expectedRating: "dislike", expectedCode: 3, expectedScore: 20},
		{score10: 0, expectedRating: "dislike", expectedCode: 3, expectedScore: 0},
	}

	for _, tt := range tests {
		rating, code, score := mapEvaluatorScore(tt.score10)
		assert.Equal(t, tt.expectedRating, rating)
		assert.Equal(t, tt.expectedCode, code)
		assert.Equal(t, tt.expectedScore, score)
	}
}
