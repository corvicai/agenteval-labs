package service

import (
	"encoding/json"
	"fmt"
)

type QuestionSetService struct {
	db any // *gorm.DB
}

type QuestionData struct {
	Notes      string     `json:"notes,omitempty"`
	Categories []Category `json:"categories"`
}

type Category struct {
	Name      string     `json:"name"`
	Questions []Question `json:"questions"`
}

type Question struct {
	ID       any    `json:"id,omitempty"`
	Question string `json:"question"`
	Expected string `json:"expected,omitempty"`
}

func (s *QuestionSetService) Import(rawData []byte) (*QuestionData, error) {
	var data QuestionData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, err
	}

	// Ensure stable IDs
	for i, cat := range data.Categories {
		for j, q := range cat.Questions {
			if q.ID == nil {
				// Generate stable ID based on position
				data.Categories[i].Questions[j].ID = fmt.Sprintf("q-%d-%d", i, j)
			}
		}
	}

	return &data, nil
}

func (s *QuestionSetService) Export(data *QuestionData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
