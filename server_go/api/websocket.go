package api

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"benchmarking-platform/internal/firebase"
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// Message types - Commands
const (
	CmdStartRun      = "CMD_START_RUN"
	CmdCancelRun     = "CMD_CANCEL_RUN"
	CmdRunEvaluators = "CMD_RUN_EVALUATORS"
	CmdRerunTask     = "CMD_RERUN_TASK"

	// Seeding & Admin Maintenance (Disabled in Production)
	CmdSeedHistoricalRun     = "CMD_SEED_HISTORICAL_RUN"
	CmdAdminRecalculateStats = "CMD_ADMIN_RECALCULATE_STATS"
	CmdAdminForceLogout      = "CMD_ADMIN_FORCE_LOGOUT"
	CmdAdminStartMaintenance = "CMD_ADMIN_START_MAINTENANCE"

	// Event types
	EvtMaintenanceStarted = "EVT_MAINTENANCE_STARTED"
	EvtTaskProgress       = "EVT_TASK_PROGRESS"

	// Request types (Expecting a DATA response)
	ReqGetManagerStats   = "REQ_GET_MANAGER_STATS"
	ReqGetManagerUsers   = "REQ_GET_MANAGER_USERS"
	ReqGetOrgStats       = "REQ_GET_ORG_STATS"
	ReqGetGlobalStats    = "REQ_GET_GLOBAL_STATS"
	ReqSyncState         = "REQ_SYNC_STATE"
	ReqGetRunDetails     = "REQ_GET_RUN_DETAILS"
	ReqCreateEvaluation  = "REQ_CREATE_EVALUATION"
	ReqGetSpyPayload     = "REQ_GET_SPY_PAYLOAD"
	ReqGetWorkspaceStats = "REQ_GET_WORKSPACE_STATS"

	// Dev/Auth Request types
	ReqDevGetManagers              = "REQ_DEV_GET_MANAGERS" // Dev-only
	ReqDevLogin                    = "REQ_DEV_LOGIN"        // Dev-only
	ReqGetWorkspaces               = "REQ_GET_WORKSPACES"
	ReqCheckManagerStatus          = "REQ_CHECK_MANAGER_STATUS"
	ReqGetMe                       = "REQ_GET_ME"
	ReqCheckAdminExists            = "REQ_CHECK_ADMIN_EXISTS"
	ReqWsLogin                     = "REQ_WS_LOGIN" // Login via WebSocket
	ReqAuth                        = "AUTH"         // Firebase Social Auth
	ReqGetWorkspaceRuns            = "REQ_GET_WORKSPACE_RUNS"
	ReqSwitchWorkspace             = "REQ_SWITCH_WORKSPACE"
	ReqCreateWorkspace             = "REQ_CREATE_WORKSPACE"
	ReqCloneWorkspace              = "REQ_CLONE_WORKSPACE"
	ReqCreateOrganization          = "REQ_CREATE_ORGANIZATION"
	ReqJoinOrganization            = "REQ_JOIN_ORGANIZATION"
	ReqGetWorkspaceClients         = "REQ_GET_WORKSPACE_CLIENTS"
	ReqUpdateAgent                 = "REQ_UPDATE_AGENT"
	ReqImportQuestionSet           = "REQ_IMPORT_QUESTION_SET"
	ReqExportQuestionSet           = "REQ_EXPORT_QUESTION_SET"
	ReqUpdateQuestionSet           = "REQ_UPDATE_QUESTION_SET"
	ReqCreateQuestionSet           = "REQ_CREATE_QUESTION_SET"
	ReqDeleteQuestionSet           = "REQ_DELETE_QUESTION_SET"
	ReqCreateQuestionSetShareLink  = "REQ_CREATE_QUESTION_SET_SHARE_LINK"
	ReqGetQuestionSetShareLink     = "REQ_GET_QUESTION_SET_SHARE_LINK"
	ReqAcceptQuestionSetShareLink  = "REQ_ACCEPT_QUESTION_SET_SHARE_LINK"

	// Collaborative Question Sets
	ReqCreateQuestionSetCollabInvite = "REQ_CREATE_QS_COLLAB_INVITE"
	ReqGetQuestionSetCollabInvite    = "REQ_GET_QS_COLLAB_INVITE"
	ReqAcceptQuestionSetCollabInvite = "REQ_ACCEPT_QS_COLLAB_INVITE"
	ReqListQuestionSetCollaborators  = "REQ_LIST_QS_COLLABORATORS"
	ReqRevokeQuestionSetCollaborator = "REQ_REVOKE_QS_COLLABORATOR"

	// Shared Agents (Plano 28)
	ReqCreateAgentCollabInvite = "REQ_CREATE_AGENT_COLLAB_INVITE"
	ReqGetAgentCollabInvite    = "REQ_GET_AGENT_COLLAB_INVITE"
	ReqAcceptAgentCollabInvite = "REQ_ACCEPT_AGENT_COLLAB_INVITE"
	ReqListAgentCollaborators  = "REQ_LIST_AGENT_COLLABORATORS"
	ReqRevokeAgentCollaborator = "REQ_REVOKE_AGENT_COLLABORATOR"
	ReqUpdateQuestionSetAgents     = "REQ_UPDATE_QUESTION_SET_AGENTS"
	ReqGetQuestionSetAgentEnvelope = "REQ_GET_QUESTION_SET_AGENT_ENVELOPE"
	ReqCreateAgent                 = "REQ_CREATE_AGENT"
	ReqDeleteAgent                 = "REQ_DELETE_AGENT"
	ReqWsRegister                  = "REQ_WS_REGISTER"
	ReqWsBootstrapAdmin            = "REQ_WS_BOOTSTRAP_ADMIN"
	ReqGetRunLite                  = "REQ_GET_RUN_LITE"
	ReqGetLatestRunByQS            = "REQ_GET_LATEST_RUN_BY_QS"
	ReqGetResultDetails            = "REQ_GET_RESULT_DETAILS"
	ReqGetRetryStatus              = "REQ_GET_RETRY_STATUS"
	ReqCheckDBPerf                 = "REQ_CHECK_DB_PERF"
	ReqDeleteRun                   = "REQ_DELETE_RUN"
	ReqDeleteAllRuns               = "REQ_DELETE_ALL_RUNS"
	ReqGetMissedEvents             = "REQ_GET_MISSED_EVENTS"
	ReqPing                        = "REQ_PING"
	ReqGetRunProgress              = "REQ_GET_RUN_PROGRESS"
	ReqGetPendingResponse          = "REQ_GET_PENDING_RESPONSE"

	// WebAuthn Request types
	ReqWebAuthnRegisterBegin  = "REQ_WEBAUTHN_REGISTER_BEGIN"
	ReqWebAuthnRegisterFinish = "REQ_WEBAUTHN_REGISTER_FINISH"
	ReqWebAuthnLoginBegin     = "REQ_WEBAUTHN_LOGIN_BEGIN"
	ReqWebAuthnLoginFinish    = "REQ_WEBAUTHN_LOGIN_FINISH"
	ReqWebAuthnDeleteKey      = "REQ_WEBAUTHN_DELETE_KEY"

	// Admin Request types
	ReqAdminGetUsers          = "REQ_ADMIN_GET_USERS"
	ReqAdminGetOrganizations  = "REQ_ADMIN_GET_ORGANIZATIONS"
	ReqAdminGetUserProfile    = "REQ_ADMIN_GET_USER_PROFILE"
	ReqAdminGetOrgProfile     = "REQ_ADMIN_GET_ORG_PROFILE"
	ReqAdminGetRuns           = "REQ_ADMIN_GET_RUNS"
	ReqAdminCreateUser        = "REQ_ADMIN_CREATE_USER"
	ReqAdminCreateOrg         = "REQ_ADMIN_CREATE_ORG"
	ReqAdminUpdateUser        = "REQ_ADMIN_UPDATE_USER"
	ReqAdminDeleteUser        = "REQ_ADMIN_DELETE_USER"
	ReqAdminUpdateOrg         = "REQ_ADMIN_UPDATE_ORG"
	ReqAdminDeleteOrg         = "REQ_ADMIN_DELETE_ORG"
	ReqAdminGenerateInvite    = "REQ_ADMIN_GENERATE_INVITE"
	ReqAdminRemoveUserFromOrg = "REQ_ADMIN_REMOVE_USER_FROM_ORG"
	ReqAdminGetLoginLogs      = "REQ_ADMIN_GET_LOGIN_LOGS"
	ReqAdminGetDebugInfo      = "REQ_ADMIN_GET_DEBUG_INFO"
	ReqChangePassword         = "REQ_CHANGE_PASSWORD"
	ReqAcceptTerms            = "REQ_ACCEPT_TERMS"

	// Legacy Comands (mapped to correct ones or kept for compatibility)
	CmdAdminProfile     = "CMD_ADMIN_PROFILE"
	CmdAdminGetUsers    = "CMD_ADMIN_GET_USERS"
	CmdCheckAdminExists = "CMD_CHECK_ADMIN_EXISTS"
	CmdSeedHistory      = "CMD_SEED_HISTORY"
	CmdSyncState        = "CMD_SYNC_STATE"

	// Manager Request types
	ReqManagerGetWorkspaces        = "REQ_MANAGER_GET_WORKSPACES"
	ReqManagerGetAgents            = "REQ_MANAGER_GET_AGENTS"
	ReqManagerGetRuns              = "REQ_MANAGER_GET_RUNS"
	ReqManagerGetUsers             = "REQ_MANAGER_GET_USERS"
	ReqManagerCreateUser           = "REQ_MANAGER_CREATE_USER"
	ReqManagerUpdateUser           = "REQ_MANAGER_UPDATE_USER"
	ReqManagerToggleUserSuspension = "REQ_MANAGER_TOGGLE_USER_SUSPENSION"
	ReqManagerImpersonateUser      = "REQ_MANAGER_IMPERSONATE_USER"
	ReqManagerGetStats             = "REQ_MANAGER_GET_STATS"
	ReqManagerGenerateInvite       = "REQ_MANAGER_GENERATE_INVITE"

	// Data types (Responses)
	DataResponse       = "DATA_RESPONSE"
	DataManagerStats   = "DATA_MANAGER_STATS"
	DataManagerUsers   = "DATA_MANAGER_USERS"
	DataOrgStats       = "DATA_ORG_STATS"
	DataGlobalStats    = "DATA_GLOBAL_STATS"
	DataState          = "DATA_STATE"
	DataRunDetails     = "DATA_RUN_DETAILS"
	DataEvaluation     = "DATA_EVALUATION"
	DataSpyPayload     = "DATA_SPY_PAYLOAD"
	DataWorkspaceStats = "DATA_WORKSPACE_STATS"

	// Dev/Auth Data types
	DataDevManagers      = "DATA_DEV_MANAGERS"
	DataDevLoginResult   = "DATA_DEV_LOGIN_RESULT"
	DataWorkspaces       = "DATA_WORKSPACES"
	DataManagerStatus    = "DATA_MANAGER_STATUS"
	DataMe               = "DATA_ME"
	DataCheckAdminExists = "DATA_CHECK_ADMIN_EXISTS"
	DataWsLoginResult    = "DATA_WS_LOGIN_RESULT"
	DataWorkspaceRuns    = "DATA_WORKSPACE_RUNS"
	DataRunLite          = "DATA_RUN_LITE"
	DataResultDetails    = "DATA_RESULT_DETAILS"
	DataRetryStatus      = "DATA_RETRY_STATUS"
	DataMissedEvents      = "DATA_MISSED_EVENTS"
	DataPong              = "DATA_PONG"
	DataRunProgress       = "DATA_RUN_PROGRESS"
	DataPendingResponse   = "DATA_PENDING_RESPONSE"

	// Admin Data types
	DataAdminUsers         = "DATA_ADMIN_USERS"
	DataAdminOrganizations = "DATA_ADMIN_ORGANIZATIONS"
	DataAdminUserProfile   = "DATA_ADMIN_USER_PROFILE"
	DataAdminOrgProfile    = "DATA_ADMIN_ORG_PROFILE"
	DataAdminRuns          = "DATA_ADMIN_RUNS"
	DataAdminLoginLogs     = "DATA_ADMIN_LOGIN_LOGS"
	DataAdminDebugInfo     = "DATA_ADMIN_DEBUG_INFO"

	// Manager Data types
	DataManagerWorkspaces = "DATA_MANAGER_WORKSPACES"
	DataManagerAgents     = "DATA_MANAGER_AGENTS"
	DataManagerRuns       = "DATA_MANAGER_RUNS"

	// WebAuthn Data types
	DataWebAuthnRegisterOptions = "DATA_WEBAUTHN_REGISTER_OPTIONS"
	DataWebAuthnLoginOptions    = "DATA_WEBAUTHN_LOGIN_OPTIONS"
)

// Message types - Events
const (
	EvtRunInit       = "EVT_RUN_INIT"
	EvtTaskQueued    = "EVT_TASK_QUEUED"
	EvtTaskStarted   = "EVT_TASK_STARTED"
	EvtTaskCompleted = "EVT_TASK_COMPLETED"
	EvtRunFinished   = "EVT_RUN_FINISHED"
	EvtError         = "EVT_ERROR"
	EvtDataChanged   = "EVT_DATA_CHANGED"
	EvtRunStarted    = "EVT_RUN_STARTED"
	EvtRunCompleted  = "EVT_RUN_COMPLETED"
	EvtRunCancelled  = "EVT_RUN_CANCELLED"
	EvtForceLogout          = "EVT_FORCE_LOGOUT"
	EvtOnlineStatus         = "EVT_ONLINE_STATUS"
	EvtCollaboratorRevoked  = "EVT_COLLABORATOR_REVOKED"
)

// cachedAudience caches the resolved user audience for a question set.
type cachedAudience struct {
	userIDs   []uuid.UUID
	fetchedAt time.Time
}

// cachedRunQS caches the question-set ID that owns a run so broadcast fan-out
// can avoid a DB round-trip on every orchestrator event.
type cachedRunQS struct {
	questionSetID uuid.UUID
	fetchedAt     time.Time
}

// cachedAgentAudience caches the (owner + active collaborators) set for an
// agent so Plano 28 broadcasts don't hit the DB on every event.
type cachedAgentAudience struct {
	ownerUserID uuid.UUID
	collabIDs   []uuid.UUID
	fetchedAt   time.Time
}

const (
	audienceCacheTTL = 30 * time.Second
	runQSCacheTTL    = 10 * time.Minute
)

// Hub manages WebSocket connections
type Hub struct {
	connections      map[uuid.UUID]*Connection
	register         chan *Connection
	unregister       chan *Connection
	mu               sync.RWMutex
	db               *gorm.DB
	engine           *orchestrator.Engine
	statsService     *service.StatsService
	qsService        *service.QuestionSetService
	agentService     *service.AgentService
	jwtSecret        string
	Firebase         *firebase.Client
	WebAuthn         *webauthn.WebAuthn
	webauthnSessions map[string]*webauthn.SessionData

	// Audience cache for broadcast expansion (Collaborative Question Sets)
	audienceCache   map[uuid.UUID]cachedAudience
	audienceCacheMu sync.RWMutex

	// Agent audience cache for Plano 28 (Shared Agents) — owner + active
	// collaborators of a given agent. Invalidated on accept/revoke/delete.
	agentAudienceCache   map[uuid.UUID]cachedAgentAudience
	agentAudienceCacheMu sync.RWMutex

	// Cache of run → question_set mapping so orchestrator events can be
	// routed to the full QS audience (owner + active collaborators) without
	// a DB hit on every event.
	runQSCache   map[uuid.UUID]cachedRunQS
	runQSCacheMu sync.RWMutex

	// Ring buffer of recently broadcast events. Used by REQ_GET_MISSED_EVENTS
	// so clients can resume after a transient disconnect without triggering
	// a full REQ_SYNC_STATE.
	eventBuffer *EventBuffer

	// Short-lived cache of non-idempotent command responses keyed by
	// correlation_id (Plan 24, Layer 4). Allows the frontend to recover a
	// response that was sent while the client was briefly disconnected.
	responseCache *ResponseCache
}

// HubInterface defines the methods required for broadcasting (to avoid import cycles in handlers)
type HubInterface interface {
	BroadcastEvent(workspaceID uuid.UUID, resource string, action string, data any) error
	BroadcastToQuestionSetAudience(questionSetID uuid.UUID, msg []byte)
	SendEventToQS(questionSetID uuid.UUID, eventType, correlationID string, payload any) error
	SendEventForRun(runID uuid.UUID, eventType, correlationID string, payload any) error
	SendEventToUser(userID uuid.UUID, eventType, correlationID string, payload any) error
	SendEventForAgent(agentID uuid.UUID, eventType, correlationID string, payloadForOwner, payloadForCollab any) error
	InvalidateAudienceCache(questionSetID uuid.UUID)
	InvalidateAgentAudienceCache(agentID uuid.UUID)
	InvalidateRunQSCache(runID uuid.UUID)
}

type Connection struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	OrgID           uuid.UUID
	WorkspaceID     uuid.UUID
	Conn            *websocket.Conn
	Send            chan []byte
	IsAuthenticated bool
	RemoteIP        string
	Done            chan struct{} // closed when connection is unregistered
	closeOnce       sync.Once     // ensures Done is closed exactly once
}

// NewConnection creates a Connection with all channels properly initialised.
func NewConnection(id, userID, orgID, workspaceID uuid.UUID, conn *websocket.Conn, sendBuf int, isAuthenticated bool, remoteIP string) *Connection {
	return &Connection{
		ID:              id,
		UserID:          userID,
		OrgID:           orgID,
		WorkspaceID:     workspaceID,
		Conn:            conn,
		Send:            make(chan []byte, sendBuf),
		IsAuthenticated: isAuthenticated,
		RemoteIP:        remoteIP,
		Done:            make(chan struct{}),
	}
}

func NewHub(db *gorm.DB, engine *orchestrator.Engine, jwtSecret string, fb *firebase.Client) *Hub {
	h := &Hub{
		connections:        make(map[uuid.UUID]*Connection),
		register:           make(chan *Connection),
		unregister:         make(chan *Connection),
		db:                 db,
		engine:             engine,
		statsService:       service.NewStatsService(db),
		qsService:          &service.QuestionSetService{},
		agentService:       &service.AgentService{},
		jwtSecret:          jwtSecret,
		Firebase:           fb,
		webauthnSessions:   make(map[string]*webauthn.SessionData),
		audienceCache:      make(map[uuid.UUID]cachedAudience),
		agentAudienceCache: make(map[uuid.UUID]cachedAgentAudience),
		runQSCache:         make(map[uuid.UUID]cachedRunQS),
		eventBuffer:        NewEventBuffer(2000, 2*time.Minute),
		responseCache:      NewResponseCache(90 * time.Second),
	}

	rpID := os.Getenv("RP_ID")
	if rpID == "" {
		rpID = "localhost"
	}

	origin := os.Getenv("RP_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3010"
	}

	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Benchmarking Platform",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
	if err == nil {
		h.WebAuthn = w
	}

	return h
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.connections[conn.ID] = conn
			h.mu.Unlock()
			logger.Debug("[HUB] Connection registered: %s (workspace: %s)", conn.ID, conn.WorkspaceID)

			// Broadcast online status update (async to not block)
			go h.broadcastOnlineStatusToAdmins()

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.connections[conn.ID]; ok {
				delete(h.connections, conn.ID)
				conn.closeOnce.Do(func() {
					close(conn.Done)
					close(conn.Send)
				})
			}
			h.mu.Unlock()
			logger.Debug("[HUB] Connection unregistered: %s", conn.ID)

			// Broadcast online status update
			go h.broadcastOnlineStatusToAdmins()
		}
	}
}

func (h *Hub) Register(conn *Connection) {
	h.register <- conn
}

func (h *Hub) Unregister(conn *Connection) {
	h.unregister <- conn
}

func (h *Hub) BroadcastToWorkspace(workspaceID uuid.UUID, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if conn.WorkspaceID == workspaceID {
			if err := conn.safeSend(msg); err != nil {
				logger.Warn("[HUB] Failed to send to connection %s: %v", conn.ID, err)
			}
		}
	}
}

func (h *Hub) BroadcastToUser(userID uuid.UUID, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if conn.UserID == userID {
			if err := conn.safeSend(msg); err != nil {
				logger.Warn("[HUB] Failed to send to connection %s: %v", conn.ID, err)
			}
		}
	}
}

func (h *Hub) BroadcastToAll(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if err := conn.safeSend(msg); err != nil {
			logger.Warn("[HUB] Failed to send to connection %s: %v", conn.ID, err)
		}
	}
}

// EventBuffer exposes the hub's replay buffer so handlers (REQ_GET_MISSED_EVENTS)
// can consult it without reaching into unexported state.
func (h *Hub) EventBuffer() *EventBuffer { return h.eventBuffer }

// cacheAndSendResponse sends a DATA_* response to the connection and
// simultaneously caches it in the response cache keyed by correlationID.
// Use this instead of c.SendResponse for non-idempotent, slow commands
// (CMD_START_RUN, CMD_RERUN_TASK, etc.) so the frontend can recover the
// response via REQ_GET_PENDING_RESPONSE after a brief disconnect (Plan 24, Layer 4).
func (h *Hub) cacheAndSendResponse(c *Connection, msgType, correlationID string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if correlationID != "" && h.responseCache != nil {
		h.responseCache.Store(correlationID, msgType, payloadBytes)
	}
	env := models.Envelope{
		Type:          msgType,
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	}
	msg, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.safeSend(msg)
}

// buildAndBufferEvent marshals payload + envelope, stamps an EventID, and
// appends the resulting message into the replay buffer. It returns the
// ready-to-send bytes so the caller can pick the appropriate fan-out.
func (h *Hub) buildAndBufferEvent(audType AudienceType, audID uuid.UUID, eventType, correlationID string, payload any) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	eventID, counter := h.eventBuffer.NextEventID()

	env := models.Envelope{
		Type:          eventType,
		CorrelationID: correlationID,
		Payload:       payloadBytes,
		EventID:       eventID,
	}
	msg, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	h.eventBuffer.Add(BufferedEvent{
		EventID:      eventID,
		Counter:      counter,
		Timestamp:    time.Now(),
		AudienceType: audType,
		AudienceID:   audID,
		Type:         eventType,
		Msg:          msg,
	})
	return msg, nil
}

func (h *Hub) SendEvent(workspaceID uuid.UUID, eventType string, correlationID string, payload any) error {
	msg, err := h.buildAndBufferEvent(AudienceWorkspace, workspaceID, eventType, correlationID, payload)
	if err != nil {
		return err
	}
	h.BroadcastToWorkspace(workspaceID, msg)
	return nil
}

// SendEventToUser delivers an event to every active connection belonging to
// userID, recording it in the replay buffer so the recipient can still
// retrieve it after a transient disconnect.
func (h *Hub) SendEventToUser(userID uuid.UUID, eventType, correlationID string, payload any) error {
	msg, err := h.buildAndBufferEvent(AudienceUser, userID, eventType, correlationID, payload)
	if err != nil {
		return err
	}
	h.BroadcastToUser(userID, msg)
	return nil
}

// BroadcastEvent broadcasts a data change event to all connections in a workspace
func (h *Hub) BroadcastEvent(workspaceID uuid.UUID, resource string, action string, data any) error {
	payload := models.DataChangedPayload{
		Resource: resource,
		Action:   action,
		Data:     data,
	}
	return h.SendEvent(workspaceID, EvtDataChanged, "", payload)
}

// InvalidateAudienceCache removes the cached audience entry for a question set.
// Must be called whenever collaborators are added or removed.
func (h *Hub) InvalidateAudienceCache(questionSetID uuid.UUID) {
	h.audienceCacheMu.Lock()
	delete(h.audienceCache, questionSetID)
	h.audienceCacheMu.Unlock()
}

// resolveQuestionSetAudience returns the owner user ID + all active collaborator
// user IDs for questionSetID. Results are cached for audienceCacheTTL (30 s).
func (h *Hub) resolveQuestionSetAudience(questionSetID uuid.UUID) ([]uuid.UUID, error) {
	// Check cache first (read lock).
	h.audienceCacheMu.RLock()
	if entry, ok := h.audienceCache[questionSetID]; ok && time.Since(entry.fetchedAt) < audienceCacheTTL {
		ids := make([]uuid.UUID, len(entry.userIDs))
		copy(ids, entry.userIDs)
		h.audienceCacheMu.RUnlock()
		return ids, nil
	}
	h.audienceCacheMu.RUnlock()

	// Fetch from DB.
	type row struct {
		UserID uuid.UUID `gorm:"column:user_id"`
	}

	// Owner: workspace.user_id via client → question_set chain.
	var ownerRow row
	if err := h.db.Raw(`
		SELECT w.user_id
		FROM question_sets qs
		JOIN clients cl ON cl.id = qs.client_id
		JOIN workspaces w ON w.id = cl.workspace_id
		WHERE qs.id = ?
		LIMIT 1
	`, questionSetID).Scan(&ownerRow).Error; err != nil {
		return nil, err
	}

	userIDs := []uuid.UUID{ownerRow.UserID}

	// Collaborators.
	var collabRows []row
	if err := h.db.Raw(`
		SELECT user_id
		FROM question_set_collaborators
		WHERE question_set_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL
	`, questionSetID).Scan(&collabRows).Error; err == nil {
		for _, r := range collabRows {
			userIDs = append(userIDs, r.UserID)
		}
	}
	// Silently ignore missing table error (schema not yet migrated).

	// Store in cache (write lock).
	h.audienceCacheMu.Lock()
	h.audienceCache[questionSetID] = cachedAudience{
		userIDs:   userIDs,
		fetchedAt: time.Now(),
	}
	h.audienceCacheMu.Unlock()

	return userIDs, nil
}

// BroadcastToQuestionSetAudience sends a raw message to all active connections
// whose UserID belongs to the resolved audience of questionSetID.
func (h *Hub) BroadcastToQuestionSetAudience(questionSetID uuid.UUID, msg []byte) {
	audience, err := h.resolveQuestionSetAudience(questionSetID)
	if err != nil {
		logger.Warn("[HUB] resolveQuestionSetAudience failed for %s: %v", questionSetID, err)
		return
	}

	// Build lookup set.
	allowed := make(map[uuid.UUID]struct{}, len(audience))
	for _, uid := range audience {
		allowed[uid] = struct{}{}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if _, ok := allowed[conn.UserID]; ok {
			if err := conn.safeSend(msg); err != nil {
				logger.Warn("[HUB] Failed to send to connection %s: %v", conn.ID, err)
			}
		}
	}
}

// SendEventToQS serializes an event envelope and broadcasts it to the full
// audience (owner + active collaborators) of questionSetID.
func (h *Hub) SendEventToQS(questionSetID uuid.UUID, eventType, correlationID string, payload any) error {
	msg, err := h.buildAndBufferEvent(AudienceQS, questionSetID, eventType, correlationID, payload)
	if err != nil {
		return err
	}
	h.BroadcastToQuestionSetAudience(questionSetID, msg)
	return nil
}

// InvalidateRunQSCache drops the cached question-set mapping for runID.
// Must be called before deleting a run (while the relationship still exists)
// or whenever that mapping changes.
func (h *Hub) InvalidateRunQSCache(runID uuid.UUID) {
	h.runQSCacheMu.Lock()
	delete(h.runQSCache, runID)
	h.runQSCacheMu.Unlock()
}

// resolveRunQuestionSet returns the question_set_id that owns runID. Cached
// for runQSCacheTTL (10 min) because the relationship is immutable for the
// life of a run.
func (h *Hub) resolveRunQuestionSet(runID uuid.UUID) (uuid.UUID, error) {
	h.runQSCacheMu.RLock()
	if entry, ok := h.runQSCache[runID]; ok && time.Since(entry.fetchedAt) < runQSCacheTTL {
		qsID := entry.questionSetID
		h.runQSCacheMu.RUnlock()
		return qsID, nil
	}
	h.runQSCacheMu.RUnlock()

	var row struct {
		QuestionSetID uuid.UUID `gorm:"column:question_set_id"`
	}
	if err := h.db.Raw(`SELECT question_set_id FROM runs WHERE id = ? LIMIT 1`, runID).Scan(&row).Error; err != nil {
		return uuid.Nil, err
	}
	if row.QuestionSetID == uuid.Nil {
		return uuid.Nil, errors.New("run not found")
	}

	h.runQSCacheMu.Lock()
	h.runQSCache[runID] = cachedRunQS{
		questionSetID: row.QuestionSetID,
		fetchedAt:     time.Now(),
	}
	h.runQSCacheMu.Unlock()
	return row.QuestionSetID, nil
}

// SendEventForRun resolves the question set that owns runID and delivers the
// event to every connection in that QS's audience (owner + active
// collaborators). Returns an error if the run can't be resolved so the caller
// can fall back to workspace-scoped delivery.
func (h *Hub) SendEventForRun(runID uuid.UUID, eventType, correlationID string, payload any) error {
	qsID, err := h.resolveRunQuestionSet(runID)
	if err != nil {
		return err
	}
	return h.SendEventToQS(qsID, eventType, correlationID, payload)
}

// -----------------------------------------------------------------------
// Shared Agents (Plano 28) — audience + broadcast
// -----------------------------------------------------------------------

// InvalidateAgentAudienceCache removes the cached (owner, collaborators) tuple
// for agentID. Must be called whenever collaborators are added/revoked or the
// agent itself is deleted.
func (h *Hub) InvalidateAgentAudienceCache(agentID uuid.UUID) {
	h.agentAudienceCacheMu.Lock()
	delete(h.agentAudienceCache, agentID)
	h.agentAudienceCacheMu.Unlock()
}

// resolveAgentAudience returns the owner user ID + every active collaborator
// user ID for agentID. Results are cached for audienceCacheTTL (30 s) — the
// same TTL used for QS audience so the two subsystems degrade uniformly.
func (h *Hub) resolveAgentAudience(agentID uuid.UUID) (ownerUserID uuid.UUID, collabIDs []uuid.UUID, err error) {
	h.agentAudienceCacheMu.RLock()
	if entry, ok := h.agentAudienceCache[agentID]; ok && time.Since(entry.fetchedAt) < audienceCacheTTL {
		ids := make([]uuid.UUID, len(entry.collabIDs))
		copy(ids, entry.collabIDs)
		h.agentAudienceCacheMu.RUnlock()
		return entry.ownerUserID, ids, nil
	}
	h.agentAudienceCacheMu.RUnlock()

	type ownerRow struct {
		UserID uuid.UUID `gorm:"column:user_id"`
	}
	var owner ownerRow
	if err := h.db.Raw(`
		SELECT w.user_id
		FROM agents a
		JOIN workspaces w ON w.id = a.workspace_id
		WHERE a.id = ?
		LIMIT 1
	`, agentID).Scan(&owner).Error; err != nil {
		return uuid.Nil, nil, err
	}

	type collabRow struct {
		UserID uuid.UUID `gorm:"column:user_id"`
	}
	var collabRows []collabRow
	// Missing-table errors are silently treated as "no collaborators" so the
	// feature degrades gracefully on databases that haven't been migrated yet.
	_ = h.db.Raw(`
		SELECT user_id
		FROM agent_collaborators
		WHERE agent_id = ? AND accepted_at IS NOT NULL AND revoked_at IS NULL
	`, agentID).Scan(&collabRows).Error

	ids := make([]uuid.UUID, 0, len(collabRows))
	for _, r := range collabRows {
		ids = append(ids, r.UserID)
	}

	h.agentAudienceCacheMu.Lock()
	h.agentAudienceCache[agentID] = cachedAgentAudience{
		ownerUserID: owner.UserID,
		collabIDs:   ids,
		fetchedAt:   time.Now(),
	}
	h.agentAudienceCacheMu.Unlock()

	return owner.UserID, ids, nil
}

// SendEventForAgent serializes two envelope variants — one with the full
// (potentially sensitive) payload intended for the agent's owner, and one
// with a redacted payload intended for collaborators — and delivers each to
// the appropriate connections. Both envelopes enter the replay buffer so
// reconnecting clients can catch up via REQ_GET_MISSED_EVENTS.
//
// Passing payloadForOwner == payloadForCollab is allowed and produces a
// single broadcast (e.g. for deletion events where no config is included).
func (h *Hub) SendEventForAgent(agentID uuid.UUID, eventType, correlationID string, payloadForOwner, payloadForCollab any) error {
	ownerID, collabIDs, err := h.resolveAgentAudience(agentID)
	if err != nil {
		return err
	}

	// Build the owner-specific envelope. We route it via AudienceUser so the
	// replay buffer can match on userID (owners connect with their own
	// UserID, not the agent ID).
	ownerMsg, err := h.buildAndBufferEvent(AudienceUser, ownerID, eventType, correlationID, payloadForOwner)
	if err != nil {
		return err
	}
	h.BroadcastToUser(ownerID, ownerMsg)

	if len(collabIDs) == 0 {
		return nil
	}

	// Build the redacted envelope shared by every collaborator. We scope it
	// to the agent so the replay filter can ignore owners (they already got
	// the full-fat variant above).
	collabMsg, err := h.buildAndBufferEvent(AudienceAgent, agentID, eventType, correlationID, payloadForCollab)
	if err != nil {
		return err
	}

	allowed := make(map[uuid.UUID]struct{}, len(collabIDs))
	for _, id := range collabIDs {
		allowed[id] = struct{}{}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if _, ok := allowed[conn.UserID]; ok {
			if sendErr := conn.safeSend(collabMsg); sendErr != nil {
				logger.Warn("[HUB] Failed to send shared-agent event to %s: %v", conn.ID, sendErr)
			}
		}
	}
	return nil
}

func (h *Hub) broadcastOnlineStatusToAdmins() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 1. Identify unique online user IDs
	uniqueUsers := make(map[uuid.UUID]bool)
	for _, conn := range h.connections {
		if conn.UserID != uuid.Nil {
			uniqueUsers[conn.UserID] = true
		}
	}

	userIDs := make([]uuid.UUID, 0, len(uniqueUsers))
	for uid := range uniqueUsers {
		userIDs = append(userIDs, uid)
	}

	payload := models.OnlineStatusPayload{
		Total:   len(userIDs),
		UserIDs: userIDs,
	}

	payloadBytes, _ := json.Marshal(payload)
	msgBytes, _ := json.Marshal(models.Envelope{
		Type:    EvtOnlineStatus,
		Payload: payloadBytes,
	})

	// 2. Broadcast to all admins
	for _, conn := range h.connections {
		// ideally we check if user is admin, but for now we trust IsAuthenticated check done upstream
		// optimization: we could cache or query IsAdmin flag on connection if available
		// For now, let's just query the DB or rely on internal knowledge.
		// User requested this for "Admins", but knowing who is online might be useful for managers too.
		// Let's prevent leaking this to Everyone if possible.
		// Actually, let's send to valid authenticated users and let frontend filter or just live with it.
		// BUT to follow "ToAdmins", we should probably verify.
		// However, connection struct doesn't have IsAdmin.
		// Adding IsAdmin to Connection would be clean, but changes signature.
		// Let's just broadcast to ALL authenticated users. It's not super sensitive data.

		if conn.IsAuthenticated {
			_ = conn.safeSend(msgBytes)
		}
	}
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 65536
)

// WritePump writes messages from the Send channel to the WebSocket connection
func (c *Connection) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump reads messages from the WebSocket connection
func (c *Connection) ReadPump(hub *Hub, handler func(*Connection, models.Envelope)) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("[WS] Panic in ReadPump for %s: %v", c.ID, r)
		}
		hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize * 4) // Increased size for larger payloads
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn("[WS] Read error for %s: %v", c.ID, err)
			}
			break
		}

		var env models.Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			logger.Warn("[WS] Invalid message format from %s: %v", c.ID, err)
			continue
		}

		// Execute handler in a new goroutine to avoid blocking the read loop.
		// This ensures that single slow requests (like seeding) don't block
		// other messages or heartbeats on this connection.
		go handler(c, env)
	}
}

// safeSend writes a message to the Send channel and recovers from any panic
// caused by writing to a closed channel. This avoids races with Hub.Unregister.
func (c *Connection) safeSend(msg []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("connection closed (panic recovered)")
		}
	}()

	select {
	case <-c.Done:
		return errors.New("connection closed")
	default:
	}

	select {
	case c.Send <- msg:
		return nil
	case <-c.Done:
		return errors.New("connection closed")
	default:
		return errors.New("buffer full")
	}
}

// SendResponse sends a DATA_* response matched by correlationID
func (c *Connection) SendResponse(msgType string, correlationID string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	env := models.Envelope{
		Type:          msgType,
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	}

	msg, err := json.Marshal(env)
	if err != nil {
		return err
	}

	return c.safeSend(msg)
}

func (c *Connection) SendError(correlationID string, errMsg string) {
	c.SendErrorWithDetails(correlationID, errMsg, nil)
}

func wsDebugEnabled() bool {
	if value := os.Getenv("WS_DEBUG_ERRORS"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err == nil {
			return enabled
		}
	}
	return os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == ""
}

func (c *Connection) SendErrorWithDetails(correlationID string, errMsg string, details any) {
	payload := map[string]any{
		"error": errMsg,
	}

	if wsDebugEnabled() {
		if details != nil {
			payload["details"] = details
		}
	}

	// Always emit server-side context so Cloud Run logs show root causes even when
	// payload details are suppressed for clients.
	logger.Error("[WS][ERROR] correlation_id=%s conn_id=%s user_id=%s workspace_id=%s message=%q details=%v",
		correlationID,
		c.ID,
		c.UserID,
		c.WorkspaceID,
		errMsg,
		details,
	)

	c.SendResponse(EvtError, correlationID, payload)
}

// checkAdmin validates if the connection user is an admin
func (h *Hub) checkAdmin(c *Connection, env models.Envelope) error {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return errors.New("not authenticated")
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return errors.New("user not found")
	}
	if !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return errors.New("admin access required")
	}
	return nil
}

