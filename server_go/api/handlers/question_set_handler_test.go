package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"benchmarking-platform/models"
)

func setupQuestionSetTestDB(t *testing.T) (*gorm.DB, uuid.UUID) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Workspace{},
		&models.Client{},
		&models.QuestionSet{},
	)
	require.NoError(t, err)

	// Create user -> workspace -> client chain
	user := models.User{ID: uuid.New(), Name: "Test", Email: "test@test.com"}
	db.Create(&user)

	workspace := models.Workspace{ID: uuid.New(), UserID: user.ID, Name: "Test WS"}
	db.Create(&workspace)

	client := models.Client{ID: uuid.New(), WorkspaceID: workspace.ID, Name: "Test Client"}
	db.Create(&client)

	return db, client.ID
}

func TestQuestionSetHandler_Import(t *testing.T) {
	db, clientID := setupQuestionSetTestDB(t)
	handler := NewQuestionSetHandler(db, &MockHub{})

	e := echo.New()

	t.Run("imports with stable ID generation", func(t *testing.T) {
		reqBody := CreateQuestionSetRequest{
			Name:    "Q4 Benchmark",
			Version: "1.0",
			Data: QuestionData{
				Notes: "Include pricing caveats in the final PDF summary.",
				Categories: []Category{
					{
						Name: "General",
						Questions: []Question{
							{Question: "What is 2+2?", Expected: "4"},
							{Question: "What is the capital of France?"},
						},
					},
					{
						Name: "Technical",
						Questions: []Question{
							{ID: "custom-id", Question: "Explain Docker"},
						},
					},
				},
			},
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("client_id")
		c.SetParamValues(clientID.String())

		err := handler.Import(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		// Verify stored data
		var qs models.QuestionSet
		db.First(&qs, "name = ?", "Q4 Benchmark")

		var data QuestionData
		json.Unmarshal(qs.Data, &data)

		assert.Equal(t, "Include pricing caveats in the final PDF summary.", data.Notes)

		// First question should have auto-generated ID (json numbers are float64)
		assert.Equal(t, float64(0), data.Categories[0].Questions[0].ID)
		assert.Equal(t, float64(1), data.Categories[0].Questions[1].ID)

		// Custom ID should be preserved
		assert.Equal(t, "custom-id", data.Categories[1].Questions[0].ID)
	})

	t.Run("preserves order", func(t *testing.T) {
		reqBody := CreateQuestionSetRequest{
			Name: "Order Test",
			Data: QuestionData{
				Categories: []Category{
					{
						Name: "Cat A",
						Questions: []Question{
							{Question: "Q1"},
							{Question: "Q2"},
							{Question: "Q3"},
						},
					},
				},
			},
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("client_id")
		c.SetParamValues(clientID.String())

		err := handler.Import(c)
		assert.NoError(t, err)

		var qs models.QuestionSet
		db.First(&qs, "name = ?", "Order Test")

		var data QuestionData
		json.Unmarshal(qs.Data, &data)

		// Verify order is preserved
		assert.Equal(t, "Q1", data.Categories[0].Questions[0].Question)
		assert.Equal(t, "Q2", data.Categories[0].Questions[1].Question)
		assert.Equal(t, "Q3", data.Categories[0].Questions[2].Question)
	})
}

func TestQuestionSetHandler_Export(t *testing.T) {
	db, clientID := setupQuestionSetTestDB(t)
	handler := NewQuestionSetHandler(db, &MockHub{})

	// Create a question set
	qsData := QuestionData{
		Notes: "Highlight the regression risk in the report summary.",
		Categories: []Category{
			{
				Name: "Export Test",
				Questions: []Question{
					{ID: "1", Question: "Test Q?", Expected: "Yes"},
				},
			},
		},
	}
	dataBytes, _ := json.Marshal(qsData)

	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: clientID,
		Name:     "Export Test Set",
		Data:     datatypes.JSON(dataBytes),
	}
	db.Create(&qs)

	e := echo.New()

	t.Run("exports in correct format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(qs.ID.String())

		err := handler.Export(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var exported QuestionData
		json.Unmarshal(rec.Body.Bytes(), &exported)

		assert.Equal(t, "Highlight the regression risk in the report summary.", exported.Notes)
		assert.Len(t, exported.Categories, 1)
		assert.Equal(t, "Export Test", exported.Categories[0].Name)
		assert.Equal(t, "Test Q?", exported.Categories[0].Questions[0].Question)
		assert.Equal(t, "Yes", exported.Categories[0].Questions[0].Expected)
	})
}
