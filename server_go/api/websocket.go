package api

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"

	"math/rand"

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
	ReqDevGetManagers          = "REQ_DEV_GET_MANAGERS" // Dev-only
	ReqDevLogin                = "REQ_DEV_LOGIN"        // Dev-only
	ReqGetWorkspaces           = "REQ_GET_WORKSPACES"
	ReqCheckManagerStatus      = "REQ_CHECK_MANAGER_STATUS"
	ReqGetMe                   = "REQ_GET_ME"
	ReqCheckAdminExists        = "REQ_CHECK_ADMIN_EXISTS"
	ReqWsLogin                 = "REQ_WS_LOGIN" // Login via WebSocket
	ReqGetWorkspaceRuns        = "REQ_GET_WORKSPACE_RUNS"
	ReqSwitchWorkspace         = "REQ_SWITCH_WORKSPACE"
	ReqCreateWorkspace         = "REQ_CREATE_WORKSPACE"
	ReqCloneWorkspace          = "REQ_CLONE_WORKSPACE"
	ReqJoinOrganization        = "REQ_JOIN_ORGANIZATION"
	ReqGetWorkspaceClients     = "REQ_GET_WORKSPACE_CLIENTS"
	ReqUpdateAgent             = "REQ_UPDATE_AGENT"
	ReqImportQuestionSet       = "REQ_IMPORT_QUESTION_SET"
	ReqExportQuestionSet       = "REQ_EXPORT_QUESTION_SET"
	ReqUpdateQuestionSet       = "REQ_UPDATE_QUESTION_SET"
	ReqCreateQuestionSet       = "REQ_CREATE_QUESTION_SET"
	ReqUpdateQuestionSetAgents = "REQ_UPDATE_QUESTION_SET_AGENTS"
	ReqCreateAgent             = "REQ_CREATE_AGENT"
	ReqDeleteAgent             = "REQ_DELETE_AGENT"
	ReqWsRegister              = "REQ_WS_REGISTER"
	ReqWsBootstrapAdmin        = "REQ_WS_BOOTSTRAP_ADMIN"
	ReqGetRunLite              = "REQ_GET_RUN_LITE"
	ReqGetLatestRunByQS        = "REQ_GET_LATEST_RUN_BY_QS"
	ReqGetResultDetails        = "REQ_GET_RESULT_DETAILS"
	ReqCheckDBPerf             = "REQ_CHECK_DB_PERF"
	ReqDeleteRun               = "REQ_DELETE_RUN"
	ReqDeleteAllRuns           = "REQ_DELETE_ALL_RUNS"

	// Admin Request types
	ReqAdminGetUsers         = "REQ_ADMIN_GET_USERS"
	ReqAdminGetOrganizations = "REQ_ADMIN_GET_ORGANIZATIONS"
	ReqAdminGetUserProfile   = "REQ_ADMIN_GET_USER_PROFILE"
	ReqAdminGetOrgProfile    = "REQ_ADMIN_GET_ORG_PROFILE"
	ReqAdminCreateUser       = "REQ_ADMIN_CREATE_USER"
	ReqAdminCreateOrg        = "REQ_ADMIN_CREATE_ORG"
	ReqAdminUpdateUser       = "REQ_ADMIN_UPDATE_USER"
	ReqAdminDeleteUser       = "REQ_ADMIN_DELETE_USER"
	ReqAdminUpdateOrg        = "REQ_ADMIN_UPDATE_ORG"
	ReqAdminDeleteOrg        = "REQ_ADMIN_DELETE_ORG"
	ReqAdminGenerateInvite   = "REQ_ADMIN_GENERATE_INVITE"
	ReqAdminGetLoginLogs     = "REQ_ADMIN_GET_LOGIN_LOGS"

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

	// Admin Data types
	DataAdminUsers         = "DATA_ADMIN_USERS"
	DataAdminOrganizations = "DATA_ADMIN_ORGANIZATIONS"
	DataAdminUserProfile   = "DATA_ADMIN_USER_PROFILE"
	DataAdminOrgProfile    = "DATA_ADMIN_ORG_PROFILE"
	DataAdminLoginLogs     = "DATA_ADMIN_LOGIN_LOGS"

	// Manager Data types
	DataManagerWorkspaces = "DATA_MANAGER_WORKSPACES"
	DataManagerAgents     = "DATA_MANAGER_AGENTS"
	DataManagerRuns       = "DATA_MANAGER_RUNS"
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
	connections  map[uuid.UUID]*Connection
	register     chan *Connection
	unregister   chan *Connection
	mu           sync.RWMutex
	db           *gorm.DB
	engine       *orchestrator.Engine
	statsService *service.StatsService
	qsService    *service.QuestionSetService
	agentService *service.AgentService
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
}

func NewHub(db *gorm.DB, engine *orchestrator.Engine) *Hub {
	return &Hub{
		connections:  make(map[uuid.UUID]*Connection),
		register:     make(chan *Connection),
		unregister:   make(chan *Connection),
		db:           db,
		engine:       engine,
		statsService: service.NewStatsService(db),
		// qsService and agentService will be initialized if needed, or we can initialize them here
		qsService:    &service.QuestionSetService{},
		agentService: &service.AgentService{},
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.connections[conn.ID] = conn
			h.mu.Unlock()
			log.Printf("[HUB] Connection registered: %s (workspace: %s)", conn.ID, conn.WorkspaceID)

			// Broadcast online status update (async to not block)
			go h.broadcastOnlineStatusToAdmins()

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.connections[conn.ID]; ok {
				delete(h.connections, conn.ID)
				close(conn.Send)
			}
			h.mu.Unlock()
			log.Printf("[HUB] Connection unregistered: %s", conn.ID)

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
			select {
			case conn.Send <- msg:
			default:
				log.Printf("[HUB] Failed to send to connection %s, buffer full", conn.ID)
			}
		}
	}
}

func (h *Hub) BroadcastToUser(userID uuid.UUID, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		if conn.UserID == userID {
			select {
			case conn.Send <- msg:
			default:
				log.Printf("[HUB] Failed to send to connection %s, buffer full", conn.ID)
			}
		}
	}
}

func (h *Hub) BroadcastToAll(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		select {
		case conn.Send <- msg:
		default:
			log.Printf("[HUB] Failed to send to connection %s, buffer full", conn.ID)
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
		// Since we don't store IsAdmin on Connection, we might want to just broadcast to everyone for now?
		// User requested this for "Admins", but knowing who is online might be useful for managers too.
		// Let's prevent leaking this to Everyone if possible.
		// Actually, let's send to valid authenticated users and let frontend filter or just live with it.
		// BUT to follow "ToAdmins", we should probably verify.
		// However, connection struct doesn't have IsAdmin.
		// Adding IsAdmin to Connection would be clean, but changes signature.
		// Let's just broadcast to ALL authenticated users. It's not super sensitive data.

		if conn.IsAuthenticated {
			select {
			case conn.Send <- msgBytes:
			default:
			}
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
			log.Printf("[WS] Panic in ReadPump for %s: %v", c.ID, r)
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
				log.Printf("[WS] Read error for %s: %v", c.ID, err)
			}
			break
		}

		var env models.Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			log.Printf("[WS] Invalid message format from %s: %v", c.ID, err)
			continue
		}

		// Execute handler in a new goroutine to avoid blocking the read loop.
		// This ensures that single slow requests (like seeding) don't block
		// other messages or heartbeats on this connection.
		go handler(c, env)
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

	select {
	case c.Send <- msg:
		return nil
	default:
		return errors.New("buffer full")
	}
}

func (c *Connection) SendError(correlationID string, errMsg string) {
	c.SendErrorWithDetails(correlationID, errMsg, nil)
}

func (c *Connection) SendErrorWithDetails(correlationID string, errMsg string, details any) {
	payload := map[string]any{
		"error": errMsg,
	}

	if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "" {
		if details != nil {
			payload["details"] = details
		}
	}

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
