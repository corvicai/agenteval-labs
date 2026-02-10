package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"

	"benchmarking-platform/api"
	"benchmarking-platform/api/handlers"
	"benchmarking-platform/internal/db"
	"benchmarking-platform/internal/firebase"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{"iap-bearer-token"},
	CheckOrigin: func(r *http.Request) bool {
		// Dev mode: Allow all
		if os.Getenv("APP_ENV") != "production" {
			return true
		}
		// Prod mode: Strict checking
		origin := r.Header.Get("Origin")
		allowedStr := os.Getenv("ALLOWED_ORIGINS")
		if allowedStr == "" {
			return false // Secure by default
		}
		allowedOrigins := strings.Split(allowedStr, ",")
		for _, allowed := range allowedOrigins {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		return false
	},
}

func main() {
	e := echo.New()

	// Middleware
	e.IPExtractor = func(r *http.Request) string {
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return realIP
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		return strings.Split(r.RemoteAddr, ":")[0] // Simple fallback
	}
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())

	// CORS Configuration
	corsConfig := echoMiddleware.CORSConfig{
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch},
	}

	if os.Getenv("APP_ENV") != "production" {
		log.Println("[SECURITY] Running in Development Mode - CORS: Allow All")
		corsConfig.AllowOrigins = []string{"*"}
	} else {
		allowedStr := os.Getenv("ALLOWED_ORIGINS")
		if allowedStr == "" {
			log.Fatal("[SECURITY] FATAL: ALLOWED_ORIGINS environment variable must be set in production")
		}
		log.Printf("[SECURITY] Running in Production Mode - CORS Restricted to: %s", allowedStr)
		origins := strings.Split(allowedStr, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		corsConfig.AllowOrigins = origins
	}
	e.Use(echoMiddleware.CORSWithConfig(corsConfig))

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

	// Auth secret initialization
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if os.Getenv("APP_ENV") == "production" {
			log.Fatal("[SECURITY] FATAL: JWT_SECRET environment variable must be set in production")
		}
		log.Println("[SECURITY] WARN: JWT_SECRET not set, using insecure default for development")
		jwtSecret = "dev-secret-change-in-production"
	}

	engine := orchestrator.NewEngine(database, pythonURL, workerCount)

	// Initialize Firebase Admin SDK
	fbClient, err := firebase.InitFirebase()
	if err != nil {
		log.Printf("[FIREBASE] ERROR: Failed to initialize Firebase: %v", err)
	}

	// Initialize WebSocket hub
	hub := api.NewHub(database, engine, jwtSecret, fbClient)
	go hub.Run()

	engine.SetEventCallback(func(workspaceID uuid.UUID, eventType string, correlationID string, payload any) {
		hub.SendEvent(workspaceID, eventType, correlationID, payload)
	})
	engine.Start()

	// Initialize handlers with WebSocket Hub support

	// Secret is now stable across restarts
	log.Printf("[AUTH] System session secret initialized (all previous sessions invalidated)")
	authHandler := handlers.NewAuthHandler(database, jwtSecret)

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

	// WebAuthn Public Flow
	e.POST("/auth/webauthn/login/begin", authHandler.WebAuthnLoginBegin, authRateLimiter)
	e.POST("/auth/webauthn/login/finish", authHandler.WebAuthnLoginFinish, authRateLimiter)

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
	authProtected.POST("/auth/accept-terms", authHandler.AcceptTerms)

	// WebAuthn Protected Flow
	authProtected.POST("/auth/webauthn/register/begin", authHandler.WebAuthnRegisterBegin)
	authProtected.POST("/auth/webauthn/register/finish", authHandler.WebAuthnRegisterFinish)

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
			RemoteIP:        c.RealIP(),
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
