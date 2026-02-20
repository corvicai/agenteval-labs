package main

import (
	"log"
	"net"
	"net/http"
	"net/url"
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

func hostOnly(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	if idx := strings.Index(value, ","); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}

	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
			value = parsed.Host
		}
	}

	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.Count(value, ":") == 1 {
		parts := strings.SplitN(value, ":", 2)
		value = parts[0]
	}

	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), "[]")
}

func originHost(origin string) string {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return ""
	}
	return hostOnly(parsed.Host)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{"iap-bearer-token"},
	CheckOrigin: func(r *http.Request) bool {
		// Dev mode: Allow all
		if os.Getenv("APP_ENV") != "production" {
			return true
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		// Allow non-browser clients (no Origin header) while still enforcing JWT/IAP auth later.
		if origin == "" {
			return true
		}

		originHostName := originHost(origin)
		if originHostName == "" {
			log.Printf("[WS] CheckOrigin rejected malformed origin=%q", origin)
			return false
		}

		// Allow same-origin when Host is rewritten by reverse proxies.
		forwardedHost := hostOnly(r.Header.Get("X-Forwarded-Host"))
		requestHost := hostOnly(r.Host)
		if originHostName == forwardedHost || originHostName == requestHost {
			return true
		}

		// Fallback to explicit allowlist for cross-origin cases.
		allowedStr := os.Getenv("ALLOWED_ORIGINS")
		if allowedStr == "" {
			return false // Secure by default
		}
		allowedOrigins := strings.Split(allowedStr, ",")
		for _, allowed := range allowedOrigins {
			allowed = strings.TrimSpace(allowed)
			if allowed == "" {
				continue
			}
			if strings.EqualFold(allowed, origin) || hostOnly(allowed) == originHostName {
				return true
			}
		}
		log.Printf("[WS] CheckOrigin rejected origin=%q host=%q x-forwarded-host=%q allowed=%q", origin, r.Host, r.Header.Get("X-Forwarded-Host"), allowedStr)
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
	// Initialize orchestration engine (Go runner only)
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

	// Encryption key initialization for encrypted JSON fields (agents config, etc.).
	// Keep production strict, but provide a stable dev fallback for PR/preview deployments.
	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		if os.Getenv("APP_ENV") == "production" {
			log.Fatal("[SECURITY] FATAL: ENCRYPTION_KEY environment variable must be set in production")
		}
		encryptionKey = "dev-temp-encryption-key-00000001"
		if err := os.Setenv("ENCRYPTION_KEY", encryptionKey); err != nil {
			log.Fatalf("[SECURITY] FATAL: Failed to set fallback ENCRYPTION_KEY: %v", err)
		}
		log.Println("[SECURITY] WARN: ENCRYPTION_KEY not set, using development fallback key")
	} else {
		keyLen := len(encryptionKey)
		if keyLen != 16 && keyLen != 24 && keyLen != 32 {
			if os.Getenv("APP_ENV") == "production" {
				log.Fatalf("[SECURITY] FATAL: invalid ENCRYPTION_KEY length: %d bytes (must be 16, 24, or 32)", keyLen)
			}
			log.Printf("[SECURITY] WARN: invalid ENCRYPTION_KEY length (%d), using development fallback key", keyLen)
			encryptionKey = "dev-temp-encryption-key-00000001"
			if err := os.Setenv("ENCRYPTION_KEY", encryptionKey); err != nil {
				log.Fatalf("[SECURITY] FATAL: Failed to set fallback ENCRYPTION_KEY: %v", err)
			}
		}
	}

	engine := orchestrator.NewEngine(database, workerCount)
	if database != nil {
		res := database.Model(&models.Run{}).Where("status = ?", "running").Update("status", "cancelled")
		if res.Error != nil {
			log.Printf("[RUN] Failed to cancel stale runs on startup: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("[RUN] Marked %d stale running run(s) as cancelled on startup", res.RowsAffected)
		}
	}

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
	e.GET("/prompts/evaluator-system", func(c echo.Context) error {
		prompt := orchestrator.DefaultEvaluatorSystemPrompt()
		if prompt == "" {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "default evaluator prompt is empty",
			})
		}
		return c.JSON(http.StatusOK, map[string]string{
			"prompt": prompt,
		})
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
		req := c.Request()
		origin := req.Header.Get("Origin")
		forwardedHost := req.Header.Get("X-Forwarded-Host")
		forwardedProto := req.Header.Get("X-Forwarded-Proto")
		subprotocolHeader := req.Header.Get("Sec-WebSocket-Protocol")
		log.Printf(
			"[WS] Upgrade attempt host=%q origin=%q x-forwarded-host=%q x-forwarded-proto=%q subprotocol-header=%q workspace_id=%q",
			req.Host,
			origin,
			forwardedHost,
			forwardedProto,
			subprotocolHeader,
			c.QueryParam("workspace_id"),
		)

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
			log.Printf(
				"[WS] Upgrade failed host=%q origin=%q x-forwarded-host=%q err=%v",
				req.Host,
				origin,
				forwardedHost,
				err,
			)
			return err
		}
		if ws.Subprotocol() != "" {
			log.Printf("[WS] Subprotocol selected: %s", ws.Subprotocol())
		} else {
			log.Printf("[WS] Subprotocol selected: <none>")
		}

		workspaceIDStr := c.QueryParam("workspace_id")
		if workspaceIDStr != "" {
			workspaceID, _ = uuid.Parse(workspaceIDStr)
		}

		conn := api.NewConnection(
			uuid.New(),
			userID,
			orgID,
			workspaceID,
			ws,
			1024,
			isAuthenticated,
			c.RealIP(),
		)

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
	log.Printf("Runner Mode: go (in-process)")
	log.Printf("=======================================================")
	e.Logger.Fatal(e.Start(":" + port))
}
