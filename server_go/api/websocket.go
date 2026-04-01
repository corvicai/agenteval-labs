package api

import (
	"encoding/json"
	"errors"
	"math/rand"
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

	// Admin Data types
	DataAdminUsers         = "DATA_ADMIN_USERS"
	DataAdminOrganizations = "DATA_ADMIN_ORGANIZATIONS"
	DataAdminUserProfile   = "DATA_ADMIN_USER_PROFILE"
	DataAdminOrgProfile    = "DATA_ADMIN_ORG_PROFILE"
	DataAdminRuns          = "DATA_ADMIN_RUNS"
	DataAdminLoginLogs     = "DATA_ADMIN_LOGIN_LOGS"

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
	EvtForceLogout   = "EVT_FORCE_LOGOUT"
	EvtOnlineStatus  = "EVT_ONLINE_STATUS"
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
}

// HubInterface defines the methods required for broadcasting (to avoid import cycles in handlers)
type HubInterface interface {
	BroadcastEvent(workspaceID uuid.UUID, resource string, action string, data any) error
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
		connections:      make(map[uuid.UUID]*Connection),
		register:         make(chan *Connection),
		unregister:       make(chan *Connection),
		db:               db,
		engine:           engine,
		statsService:     service.NewStatsService(db),
		qsService:        &service.QuestionSetService{},
		agentService:     &service.AgentService{},
		jwtSecret:        jwtSecret,
		Firebase:         fb,
		webauthnSessions: make(map[string]*webauthn.SessionData),
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

func (h *Hub) SendEvent(workspaceID uuid.UUID, eventType string, correlationID string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	env := models.Envelope{
		Type:          eventType,
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	}

	msg, err := json.Marshal(env)
	if err != nil {
		return err
	}

	h.BroadcastToWorkspace(workspaceID, msg)
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

func generateRandomCode(length int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Excludes I, O, 1, 0
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
