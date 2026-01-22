package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"

	"benchmarking-platform/api"
	"benchmarking-platform/api/handlers"
	"benchmarking-platform/internal/db"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

func main() {
	e := echo.New()

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())
	e.Use(echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch},
	}))

	// Connect to database
	database, err := db.Connect()
	if err != nil {
		log.Printf("[WARN] Database connection failed: %v. Running without persistence.", err)
	} else {
		log.Println("[DB] Connected successfully")
		if err := db.AutoMigrate(database); err != nil {
			log.Printf("[WARN] AutoMigrate failed: %v", err)
		}
	}
	// Initialize orchestration engine
	pythonURL := os.Getenv("PYTHON_RUNNER_URL")
	if pythonURL == "" {
		pythonURL = "http://localhost:3003"
	}

	workerCount := 50
	if wcStr := os.Getenv("ENGINE_WORKERS"); wcStr != "" {
		if wc, err := strconv.Atoi(wcStr); err == nil {
			workerCount = wc
		}
	}

	engine := orchestrator.NewEngine(database, pythonURL, workerCount)

	// Initialize WebSocket hub
	hub := api.NewHub(database, engine)
	go hub.Run()

	engine.SetEventCallback(func(workspaceID uuid.UUID, eventType string, correlationID string, payload any) {
		hub.SendEvent(workspaceID, eventType, correlationID, payload)
	})
	engine.Start()

	// Initialize handlers with WebSocket Hub support
	/*
		agentHandler := handlers.NewAgentHandler(database, hub)
		qsHandler := handlers.NewQuestionSetHandler(database, hub)
		runHandler := handlers.NewRunHandler(database, engine, hub)
		evalHandler := handlers.NewEvaluationHandler(database)
		statsHandler := handlers.NewStatsHandler(database)
		orgHandler := handlers.NewOrganizationHandler(database)
	*/

	// Auth handler
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-change-in-production"
	}
	// Append random component to invalidate all sessions on restart/rebuild
	jwtSecret = jwtSecret + "-" + uuid.New().String()
	log.Printf("[AUTH] System session secret initialized (all previous sessions invalidated)")
	authHandler := handlers.NewAuthHandler(database, jwtSecret)
	// managerHandler := handlers.NewManagerHandler(database, jwtSecret)

	// ========== REST API Routes ==========

	// Public Routes
	// Public Routes with Rate Limiting (20 requests per minute)
	authRateLimiter := echoMiddleware.RateLimiter(echoMiddleware.NewRateLimiterMemoryStore(20))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.POST("/auth/register", authHandler.Register, authRateLimiter)
	e.POST("/auth/login", authHandler.Login, authRateLimiter)
	e.POST("/auth/bootstrap-admin", authHandler.BootstrapAdmin)
	e.GET("/auth/check-admin", authHandler.CheckAdminExists)

	// Dev-only routes
	if os.Getenv("APP_ENV") != "production" {
		e.GET("/auth/managers", authHandler.GetManagers) // Dev Quick Login
	}

	// Protected Routes
	config := middleware.DefaultJWTConfig()
	config.SecretKey = jwtSecret

	// Auth & Workspace Management (Protected, No Workspace Scope)
	authProtected := e.Group("")
	authProtected.Use(middleware.JWTMiddleware(config))
	authProtected.GET("/auth/me", authHandler.Me)
	authProtected.POST("/auth/refresh", authHandler.RefreshToken)
	authProtected.POST("/auth/logout", authHandler.Logout)
	authProtected.POST("/auth/join-organization", authHandler.JoinOrganization)
	authProtected.POST("/auth/select-organization", authHandler.SelectOrganization)
	/*
		authProtected.GET("/auth/workspaces", authHandler.ListWorkspaces)
		authProtected.POST("/auth/workspaces", authHandler.CreateWorkspace)
		authProtected.POST("/auth/workspaces/:workspace_id/switch", authHandler.SwitchWorkspace)
		authProtected.GET("/auth/organizations", authHandler.ListOrganizations)
		authProtected.POST("/auth/organizations/:org_id/audit-logs", authHandler.ToggleAuditLogs)
		authProtected.GET("/auth/check-manager", authHandler.CheckManagerStatus)

		// Admin (Protected)
		authProtected.GET("/admin/users", authHandler.ListUsers)
		authProtected.POST("/admin/users", authHandler.CreateUserAdmin)
		authProtected.PUT("/admin/users/:user_id", authHandler.UpdateUser)
		authProtected.DELETE("/admin/users/:user_id", authHandler.DeleteUser)
	*/

	/*
		// Organization Admin
		authProtected.GET("/admin/organizations", orgHandler.ListOrganizationsAdmin)
		authProtected.POST("/admin/organizations", orgHandler.CreateOrganization)
		authProtected.PUT("/admin/organizations/:id", orgHandler.UpdateOrganization)
		authProtected.DELETE("/admin/organizations/:id", orgHandler.DeleteOrganization)
		authProtected.GET("/admin/organizations/:id/profile", orgHandler.GetOrgProfile)

		// User Profile (admin only)
		authProtected.GET("/admin/users/:id/profile", authHandler.GetUserProfile)

		// Manager Routes (org managers only)
		authProtected.GET("/manager/users", managerHandler.GetOrgUsers)
		authProtected.POST("/manager/users", managerHandler.CreateOrgUser)
		authProtected.PUT("/manager/users/:user_id", managerHandler.UpdateOrgUser)
		authProtected.POST("/manager/users/:user_id/toggle-suspension", managerHandler.ToggleUserSuspension)
		authProtected.POST("/manager/impersonate/:user_id", managerHandler.ImpersonateUser)
		authProtected.GET("/manager/workspaces", managerHandler.GetOrgWorkspaces)
		authProtected.GET("/manager/agents", managerHandler.GetOrgAgents)
		authProtected.GET("/manager/runs", managerHandler.GetOrgRuns)
		authProtected.GET("/manager/stats", managerHandler.GetOrgStats)
	*/

	/*
		// Workspace Scoped Resources
		protected := e.Group("")
		protected.Use(middleware.JWTMiddleware(config))
		protected.Use(middleware.WorkspaceScopeMiddleware())

		// Agents (Protected)
		protected.GET("/workspaces/:workspace_id/agents", agentHandler.List)
		protected.POST("/workspaces/:workspace_id/agents", agentHandler.Create)
		protected.POST("/workspaces/:workspace_id/agents/reorder", agentHandler.Reorder)
		protected.GET("/agents/:id", agentHandler.Get)
		protected.PUT("/agents/:id", agentHandler.Update)
		protected.DELETE("/agents/:id", agentHandler.Delete)
		protected.GET("/agents/:id/spy", agentHandler.SpyPayload)

		// Question Sets (Protected)
		protected.GET("/workspaces/:workspace_id/clients", qsHandler.ListClients)
		protected.GET("/clients/:client_id/question-sets", qsHandler.List)
		protected.GET("/question-sets/:id", qsHandler.Get)
		protected.DELETE("/question-sets/:id", qsHandler.Delete)
		protected.POST("/clients/:client_id/question-sets/import", qsHandler.Import)
		protected.GET("/question-sets/:id/export", qsHandler.Export)
		protected.PUT("/question-sets/:id", qsHandler.Update)
		protected.GET("/question-sets/:id/agents", qsHandler.GetAgents)
		protected.PUT("/question-sets/:id/agents", qsHandler.UpdateAgents)

		// Runs (Protected)
		protected.POST("/workspaces/:workspace_id/runs", runHandler.StartRun)
		protected.GET("/workspaces/:workspace_id/runs", runHandler.GetWorkspaceRuns)
		protected.GET("/runs/:run_id", runHandler.GetRunStatus)   // Lightweight status
		protected.GET("/runs/:run_id/details", runHandler.GetRun) // Full details for document view
		protected.POST("/runs/:run_id/rerun", runHandler.RerunTask)
		protected.GET("/workspaces/:workspace_id/history", runHandler.GetHistory)

		// Evaluations (Protected)
		protected.POST("/evaluations", evalHandler.Create)
		protected.GET("/run-results/:run_result_id/evaluations", evalHandler.List)
		protected.DELETE("/evaluations/:id", evalHandler.Delete)

		// Run evaluators endpoint (Protected)
		protected.POST("/runs/:run_id/evaluate", func(c echo.Context) error {
			runID := c.Param("run_id")
			rID, err := uuid.Parse(runID)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
			}
			if err := engine.RunEvaluators(rID); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusAccepted, map[string]string{"status": "evaluators queued"})
		})

		// Cancel run endpoint (Protected)
		protected.POST("/runs/:run_id/cancel", func(c echo.Context) error {
			runID := c.Param("run_id")
			rID, err := uuid.Parse(runID)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
			}
			engine.CancelRun(rID)
			return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
		})

		// Stats routes
		e.GET("/stats/workspace/:workspace_id", statsHandler.GetWorkspaceStats, middleware.JWTMiddleware(config), middleware.WorkspaceScopeMiddleware())
		e.GET("/stats/organization", statsHandler.GetOrganizationStats, middleware.JWTMiddleware(config))
		e.GET("/stats/global", statsHandler.GetGlobalStats, middleware.JWTMiddleware(config))
		e.POST("/admin/stats/recalculate", statsHandler.RecalculateStats, middleware.JWTMiddleware(config))
	*/

	// ========== WebSocket ==========
	e.GET("/ws", func(c echo.Context) error {
		// Authenticate via token query param or cookie (for WS upgrade)
		// Allow anonymous connections for pre-auth endpoints
		tokenString := c.QueryParam("token")
		if tokenString == "" {
			if cookie, err := c.Cookie("token"); err == nil {
				tokenString = cookie.Value
			}
		}

		var userID, orgID, workspaceID uuid.UUID
		isAuthenticated := false

		if tokenString != "" {
			// Parse token
			token, err := jwt.ParseWithClaims(tokenString, &middleware.Claims{}, func(token *jwt.Token) (any, error) {
				return []byte(jwtSecret), nil
			})

			if err == nil && token.Valid {
				claims, _ := token.Claims.(*middleware.Claims)
				userID, _ = uuid.Parse(claims.UserID)
				orgID, _ = uuid.Parse(claims.OrgID)
				isAuthenticated = true
			}
		}

		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}

		workspaceIDStr := c.QueryParam("workspace_id")
		if workspaceIDStr != "" {
			workspaceID, _ = uuid.Parse(workspaceIDStr)
		}

		conn := &api.Connection{
			ID:              uuid.New(),
			UserID:          userID,
			OrgID:           orgID,
			WorkspaceID:     workspaceID,
			Conn:            ws,
			Send:            make(chan []byte, 1024),
			IsAuthenticated: isAuthenticated,
		}

		hub.Register(conn)

		// Start write pump
		go conn.WritePump()

		// Handle incoming messages using the new routing logic
		conn.ReadPump(hub, func(c *api.Connection, env models.Envelope) {
			hub.HandleWSMessage(c, env)
		})

		return nil
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("=======================================================")
	log.Printf("BENCHMARKING PLATFORM - Go Server")
	log.Printf("Starting on port %s", port)
	log.Printf("Python Runner URL: %s", pythonURL)
	log.Printf("=======================================================")
	e.Logger.Fatal(e.Start(":" + port))
}
