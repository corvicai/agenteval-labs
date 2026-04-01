package api

import (
	"encoding/json"
	"testing"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func decodeWSResponsePayload(t *testing.T, env *models.Envelope, target any) {
	t.Helper()
	require.NotNil(t, env)
	require.NoError(t, json.Unmarshal(env.Payload, target))
}

func createQuestionSetTestFixture(t *testing.T, userName string) (models.User, models.Workspace, models.Client, models.QuestionSet, string) {
	t.Helper()

	user := models.User{
		ID:           uuid.New(),
		Name:         userName,
		Email:        userName + "_" + uuid.NewString() + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   userName + " Workspace",
	}
	require.NoError(t, db.Create(&workspace).Error)

	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        userName + " Client",
	}
	require.NoError(t, db.Create(&client).Error)

	questionSet := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     userName + " Set",
		Version:  "1.0",
		Data: datatypes.JSON([]byte(`{
			"categories":[
				{"name":"General","questions":[
					{"id":"q1","question":"What is 2+2?","expected":"4"},
					{"id":"q2","question":"What is the capital of France?","expected":"Paris"}
				]}
			]
		}`)),
	}
	require.NoError(t, db.Create(&questionSet).Error)

	token := generateTestToken(user.ID, workspace.ID, uuid.Nil)
	return user, workspace, client, questionSet, token
}

func TestQuestionSetShareLinkLifecycle(t *testing.T) {
	setup()

	sourceUser, sourceWorkspace, _, questionSet, sourceToken := createQuestionSetTestFixture(t, "source")
	destUser, destWorkspace, _, _, destToken := createQuestionSetTestFixture(t, "dest")

	createResp := sendWSRequest(t, sourceToken, ReqCreateQuestionSetShareLink, map[string]any{
		"question_set_id": questionSet.ID.String(),
	})
	assert.Equal(t, DataResponse, createResp.Type)

	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)
	require.NotEmpty(t, created.Token)

	inspectResp := sendWSRequest(t, destToken, ReqGetQuestionSetShareLink, map[string]any{
		"token": created.Token,
	})
	assert.Equal(t, DataResponse, inspectResp.Type)

	var preview struct {
		Status          string `json:"status"`
		QuestionSetName string `json:"question_set_name"`
		QuestionCount   int    `json:"question_count"`
		SharedByName    string `json:"shared_by_name"`
	}
	decodeWSResponsePayload(t, inspectResp, &preview)
	assert.Equal(t, "ready", preview.Status)
	assert.Equal(t, questionSet.Name, preview.QuestionSetName)
	assert.Equal(t, 2, preview.QuestionCount)
	assert.Equal(t, sourceUser.Name, preview.SharedByName)

	acceptResp := sendWSRequest(t, destToken, ReqAcceptQuestionSetShareLink, map[string]any{
		"token":               created.Token,
		"target_workspace_id": destWorkspace.ID.String(),
	})
	assert.Equal(t, DataResponse, acceptResp.Type)

	var accepted struct {
		QuestionSet models.QuestionSet `json:"question_set"`
		WorkspaceID string             `json:"workspace_id"`
	}
	decodeWSResponsePayload(t, acceptResp, &accepted)
	assert.NotEqual(t, questionSet.ID, accepted.QuestionSet.ID)
	assert.Equal(t, destWorkspace.ID.String(), accepted.WorkspaceID)
	assert.Equal(t, questionSet.Name, accepted.QuestionSet.Name)
	assert.JSONEq(t, string(questionSet.Data), string(accepted.QuestionSet.Data))

	var acceptedClient models.Client
	require.NoError(t, db.First(&acceptedClient, "id = ?", accepted.QuestionSet.ClientID).Error)
	assert.Equal(t, destWorkspace.ID, acceptedClient.WorkspaceID)

	var acceptedAgentLinks int64
	require.NoError(t, db.Model(&models.QuestionSetAgent{}).
		Where("question_set_id = ?", accepted.QuestionSet.ID).
		Count(&acceptedAgentLinks).Error)
	assert.Zero(t, acceptedAgentLinks)

	var storedLink models.QuestionSetShareLink
	require.NoError(t, db.Where("token = ?", created.Token).First(&storedLink).Error)
	require.NotNil(t, storedLink.UsedAt)
	require.NotNil(t, storedLink.UsedByUserID)
	require.NotNil(t, storedLink.AcceptedQuestionSetID)
	assert.Equal(t, destUser.ID, *storedLink.UsedByUserID)
	assert.Equal(t, accepted.QuestionSet.ID, *storedLink.AcceptedQuestionSetID)

	reuseResp := sendWSRequest(t, sourceToken, ReqAcceptQuestionSetShareLink, map[string]any{
		"token":               created.Token,
		"target_workspace_id": sourceWorkspace.ID.String(),
	})
	assert.Equal(t, EvtError, reuseResp.Type)

	var reuseErr map[string]any
	decodeWSResponsePayload(t, reuseResp, &reuseErr)
	assert.Equal(t, "share link already used", reuseErr["error"])
}

func TestQuestionSetShareLinkLifecycleRepairsMissingTableOnDemand(t *testing.T) {
	setup()

	_, sourceWorkspace, _, questionSet, sourceToken := createQuestionSetTestFixture(t, "source-repair")
	_, destWorkspace, _, _, destToken := createQuestionSetTestFixture(t, "dest-repair")

	require.NoError(t, db.Migrator().DropTable(&models.QuestionSetShareLink{}))

	createResp := sendWSRequest(t, sourceToken, ReqCreateQuestionSetShareLink, map[string]any{
		"question_set_id": questionSet.ID.String(),
	})
	assert.Equal(t, DataResponse, createResp.Type)

	var created struct {
		Token string `json:"token"`
	}
	decodeWSResponsePayload(t, createResp, &created)
	require.NotEmpty(t, created.Token)
	assert.True(t, db.Migrator().HasTable(&models.QuestionSetShareLink{}))

	inspectResp := sendWSRequest(t, destToken, ReqGetQuestionSetShareLink, map[string]any{
		"token": created.Token,
	})
	assert.Equal(t, DataResponse, inspectResp.Type)

	var preview struct {
		Status string `json:"status"`
	}
	decodeWSResponsePayload(t, inspectResp, &preview)
	assert.Equal(t, "ready", preview.Status)

	acceptResp := sendWSRequest(t, destToken, ReqAcceptQuestionSetShareLink, map[string]any{
		"token":               created.Token,
		"target_workspace_id": destWorkspace.ID.String(),
	})
	assert.Equal(t, DataResponse, acceptResp.Type)

	var accepted struct {
		WorkspaceID string `json:"workspace_id"`
	}
	decodeWSResponsePayload(t, acceptResp, &accepted)
	assert.Equal(t, destWorkspace.ID.String(), accepted.WorkspaceID)

	var repairedLink models.QuestionSetShareLink
	require.NoError(t, db.Where("token = ?", created.Token).First(&repairedLink).Error)
	assert.NotNil(t, repairedLink.UsedAt)
	assert.Equal(t, questionSet.ID, repairedLink.QuestionSetID)
	assert.Equal(t, sourceWorkspace.UserID, repairedLink.CreatedByUserID)
}
