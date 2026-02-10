package service

import (
	"testing"

	"benchmarking-platform/models"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStatsService_ComputeStats(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.UserOrganization{},
		&models.Workspace{},
		&models.Agent{},
		&models.Run{},
		&models.RunResult{},
		&models.Evaluation{},
	)
	require.NoError(t, err)

	// Setup data
	orgID := uuid.New()
	db.Create(&models.Organization{ID: orgID, Name: "Test Org"})

	userID := uuid.New()
	db.Create(&models.User{ID: userID, Name: "Test User", Email: "test@test.com"})

	// Associate user with organization
	db.Create(&models.UserOrganization{UserID: userID, OrganizationID: orgID, Role: "member"})

	wsID := uuid.New()
	db.Create(&models.Workspace{ID: wsID, UserID: userID, Name: "Test WS"})

	agentID := uuid.New()
	db.Create(&models.Agent{ID: agentID, WorkspaceID: wsID, Name: "Test Agent", ProviderType: "mcp"})

	runID := uuid.New()
	db.Create(&models.Run{ID: runID, WorkspaceID: wsID, QuestionSetID: uuid.New(), Status: "completed"})

	// Result 1: Like + Score 100
	res1ID := uuid.New()
	db.Create(&models.RunResult{ID: res1ID, RunID: runID, AgentID: agentID, QuestionID: "q1", Status: "success", DurationMs: 100})
	rating1 := 1
	score1 := 100
	db.Create(&models.Evaluation{ID: uuid.New(), RunResultID: res1ID, RaterType: "user", Rating: "like", RatingCode: &rating1, Score: &score1})

	// Result 2: Dislike + Score 0
	res2ID := uuid.New()
	db.Create(&models.RunResult{ID: res2ID, RunID: runID, AgentID: agentID, QuestionID: "q2", Status: "success", DurationMs: 200})
	rating2 := 3
	score2 := 0
	db.Create(&models.Evaluation{ID: uuid.New(), RunResultID: res2ID, RaterType: "user", Rating: "dislike", RatingCode: &rating2, Score: &score2})

	service := NewStatsService(db)

	t.Run("computes workspace stats", func(t *testing.T) {
		stats, err := service.ComputeStats("workspace", &wsID)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), stats.TotalRuns)
		assert.Equal(t, int64(2), stats.TotalResults)
		assert.Equal(t, int64(2), stats.TotalEvaluations)
		assert.Equal(t, 0.5, stats.PositiveRate)
		assert.Equal(t, 0.5, stats.NegativeRate)
		assert.Equal(t, 50.0, stats.AvgScore)
		assert.Len(t, stats.Agents, 1)
		assert.Equal(t, int64(2), stats.Agents[0].TotalEvaluations)
		assert.Equal(t, 50.0, stats.Agents[0].AvgScore)
	})

	t.Run("computes organization stats", func(t *testing.T) {
		stats, err := service.ComputeStats("organization", &orgID)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), stats.TotalEvaluations)
	})

	t.Run("computes global stats", func(t *testing.T) {
		stats, err := service.ComputeStats("global", nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), stats.TotalEvaluations)
	})
}
