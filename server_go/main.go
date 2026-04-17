package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"

	"benchmarking-platform/api"
	"benchmarking-platform/api/handlers"
	"benchmarking-platform/internal/buildinfo"
	"benchmarking-platform/internal/db"
	"benchmarking-platform/internal/firebase"
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/internal/security"
	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"
	"benchmarking-platform/orchestrator"
)

type appRevisionInfo struct {
	Commit    string `json:"commit"`
	Branch    string `json:"branch,omitempty"`
	Dirty     string `json:"dirty,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func loadAppRevisionInfo() appRevisionInfo {
	return appRevisionInfo{
		Commit:    firstNonEmpty(buildinfo.Commit, os.Getenv("APP_REVISION"), os.Getenv("GIT_COMMIT")),
		Branch:    firstNonEmpty(buildinfo.Branch, os.Getenv("APP_REVISION_BRANCH")),
		Dirty:     firstNonEmpty(buildinfo.Dirty, os.Getenv("APP_REVISION_DIRTY")),
		UpdatedAt: firstNonEmpty(buildinfo.UpdatedAt, os.Getenv("APP_REVISION_UPDATED_AT")),
	}
}

// shouldBlockStartupForEncryptionHealth returns true (with reason) when production
// should refuse to start due to an unrecoverable encryption key problem.
//
// "mismatch" (key fingerprint changed) blocks by default in production unless
// ENCRYPTION_KEY_AUTO_PROMOTE=true is explicitly set. This forces operators to
// make a conscious choice: either perform a proper key rotation (recommended) or
// accept that data encrypted with the old key will become unreadable.
//
// "sentinel_failed" means the stored fingerprint matches but the sentinel ciphertext
// cannot be decrypted. This indicates key corruption or DB tampering and is always fatal.
func shouldBlockStartupForEncryptionHealth(appEnv string, health service.EncryptionKeyHealth, autoPromote bool) (bool, string) {
	if strings.TrimSpace(appEnv) != "production" {
		return false, ""
	}
	if health.StateStatus == "sentinel_failed" {
		return true, health.StateSummary
	}
	if health.StateStatus == "mismatch" && !autoPromote {
		return true, "ENCRYPTION_KEY fingerprint mismatch — the current key does not match the last known active fingerprint. " +
			"Options: (1) restore the original key, " +
			"(2) set ENCRYPTION_KEY_PREVIOUS=<old_key> + ENCRYPTION_KEY_ROTATE_ON_START=true for safe rotation (recommended), " +
			"or (3) set ENCRYPTION_KEY_AUTO_PROMOTE=true to accept that agents encrypted with the old key will need re-entering credentials"
	}
	return false, ""
}

func parseEncryptionAutoPromoteFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

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
		allowedStr := os.Getenv("ALLOWED_ORIGINS")

		// No explicit allowlist and not production: allow all (local dev).
		if allowedStr == "" && os.Getenv("APP_ENV") != "production" {
			return true
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		// Allow non-browser clients (no Origin header) while still enforcing JWT/IAP auth later.
		if origin == "" {
			return true
		}

		originHostName := originHost(origin)
		if originHostName == "" {
			logger.Warn("[WS] CheckOrigin rejected malformed origin=%q", origin)
			return false
		}

		// Allow same-origin when Host is rewritten by reverse proxies.
		forwardedHost := hostOnly(r.Header.Get("X-Forwarded-Host"))
		requestHost := hostOnly(r.Host)
		if originHostName == forwardedHost || originHostName == requestHost {
			return true
		}

		// Explicit allowlist (set in Cloud Run dev and prod).
		if allowedStr == "" {
			return false
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
		logger.Warn("[WS] CheckOrigin rejected origin=%q host=%q x-forwarded-host=%q allowed=%q", origin, r.Host, r.Header.Get("X-Forwarded-Host"), allowedStr)
		return false
	},
}

func main() {
	logger.Init()
	appRevision := loadAppRevisionInfo()
	if appRevision.Commit != "" {
		logger.Info("[BUILD] Revision commit=%s branch=%s dirty=%s updated_at=%s",
			appRevision.Commit,
			appRevision.Branch,
			appRevision.Dirty,
			appRevision.UpdatedAt,
		)
	}

	e := echo.New()

	// Middleware
	// Only trust X-Real-IP / X-Forwarded-For when running behind a trusted
	// proxy (Cloud Run, IAP, reverse proxy). In development these headers
	// are spoofable, so default to RemoteAddr unless explicitly enabled.
	trustProxyHeaders := os.Getenv("TRUST_PROXY_HEADERS") == "true" ||
		os.Getenv("APP_ENV") == "production"
	e.IPExtractor = func(r *http.Request) string {
		if trustProxyHeaders {
			if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				return realIP
			}
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				return strings.TrimSpace(strings.Split(xff, ",")[0])
			}
		}
		return strings.Split(r.RemoteAddr, ":")[0] // Simple fallback
	}
	// Structured HTTP request logging via our internal logger. Replaces the
	// deprecated echoMiddleware.Logger() which printed to stdout directly.
	e.Use(echoMiddleware.RequestLoggerWithConfig(echoMiddleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogLatency:  true,
		LogError:    true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v echoMiddleware.RequestLoggerValues) error {
			if v.Error != nil {
				logger.Error("[HTTP] %s %s status=%d latency=%s err=%v",
					v.Method, v.URI, v.Status, v.Latency, v.Error)
			} else {
				logger.Info("[HTTP] %s %s status=%d latency=%s",
					v.Method, v.URI, v.Status, v.Latency)
			}
			return nil
		},
	}))
	e.Use(echoMiddleware.Recover())

	// CORS Configuration
	corsConfig := echoMiddleware.CORSConfig{
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch},
	}

	allowedStr := os.Getenv("ALLOWED_ORIGINS")
	switch {
	case allowedStr != "":
		// Explicit allowlist: restrict CORS regardless of APP_ENV.
		// Set this in Cloud Run (both dev and prod) via Pulumi config.
		origins := strings.Split(allowedStr, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		corsConfig.AllowOrigins = origins
		logger.Info("[SECURITY] CORS restricted to: %s", allowedStr)
	case os.Getenv("APP_ENV") == "production":
		log.Fatal("[SECURITY] FATAL: ALLOWED_ORIGINS environment variable must be set in production")
	default:
		corsConfig.AllowOrigins = []string{"*"}
		logger.Info("[SECURITY] Running in local dev mode - CORS: Allow All")
	}
	e.Use(echoMiddleware.CORSWithConfig(corsConfig))

	// Connect to database
	database, err := db.Connect()
	if err != nil {
		logger.Warn("[DB] Database connection failed: %v. Running without persistence.", err)
	} else {
		logger.Info("[DB] Connected successfully")
		if err := db.AutoMigrate(database); err != nil {
			logger.Warn("[DB] AutoMigrate failed: %v", err)
		}
		if err := db.EnsureCriticalSchema(database); err != nil {
			logger.Warn("[DB] Critical schema compatibility failed: %v", err)
		}
	}
	// Initialize orchestration engine (Go runner only)
	workerCount := 50
	if wcStr := os.Getenv("ENGINE_WORKERS"); wcStr != "" {
		if wc, err := strconv.Atoi(wcStr); err == nil {
			workerCount = wc
		}
	}

	queueSize := 0
	if qsStr := os.Getenv("ENGINE_QUEUE_SIZE"); qsStr != "" {
		if qs, err := strconv.Atoi(qsStr); err == nil {
			queueSize = qs
		} else {
			logger.Warn("[ENGINE] Invalid ENGINE_QUEUE_SIZE=%q, falling back to default: %v", qsStr, err)
		}
	}

	// Auth secret initialization
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if os.Getenv("APP_ENV") == "production" {
			log.Fatal("[SECURITY] FATAL: JWT_SECRET environment variable must be set in production")
		}
		logger.Warn("[SECURITY] JWT_SECRET not set, using insecure default for development")
		jwtSecret = "dev-secret-change-in-production"
	}
	if os.Getenv("APP_ENV") == "production" && len(jwtSecret) < 32 {
		log.Fatal("[SECURITY] FATAL: JWT_SECRET must be at least 32 characters in production")
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
		security.SetEncryptionKeyRuntimeStatus(security.EncryptionKeyRuntimeStatus{
			Status:       "fallback",
			Source:       "development_fallback",
			Summary:      "Using development fallback because ENCRYPTION_KEY was not set",
			Loaded:       true,
			UsedFallback: true,
			Format:       "raw",
			CharLength:   len(encryptionKey),
			ParsedBytes:  len(encryptionKey),
		})
		logger.Warn("[SECURITY] ENCRYPTION_KEY not set, using development fallback key")
	} else {
		key, format, err := security.ParseEncryptionKey(encryptionKey)
		switch {
		case err != nil:
			if os.Getenv("APP_ENV") == "production" {
				log.Fatalf("[SECURITY] FATAL: %v", err)
			}
			logger.Warn("[SECURITY] Invalid ENCRYPTION_KEY (%v), using development fallback key", err)
			encryptionKey = "dev-temp-encryption-key-00000001"
			if err := os.Setenv("ENCRYPTION_KEY", encryptionKey); err != nil {
				log.Fatalf("[SECURITY] FATAL: Failed to set fallback ENCRYPTION_KEY: %v", err)
			}
			security.SetEncryptionKeyRuntimeStatus(security.EncryptionKeyRuntimeStatus{
				Status:          "fallback",
				Source:          "development_fallback",
				Summary:         "Using development fallback because the provided ENCRYPTION_KEY was invalid",
				Loaded:          true,
				UsedFallback:    true,
				Format:          "raw",
				CharLength:      len(encryptionKey),
				ParsedBytes:     len(encryptionKey),
				ValidationError: err.Error(),
			})
		case format == "hex":
			security.SetEncryptionKeyRuntimeStatus(security.EncryptionKeyRuntimeStatus{
				Status:      "loaded",
				Source:      "environment",
				Summary:     "ENCRYPTION_KEY loaded successfully from a hex-encoded environment value",
				Loaded:      true,
				Format:      format,
				CharLength:  len(encryptionKey),
				ParsedBytes: len(key),
			})
			logger.Warn("[SECURITY] ENCRYPTION_KEY loaded from hex-encoded secret (%d chars -> %d bytes)", len(encryptionKey), len(key))
		default:
			security.SetEncryptionKeyRuntimeStatus(security.EncryptionKeyRuntimeStatus{
				Status:      "loaded",
				Source:      "environment",
				Summary:     "ENCRYPTION_KEY loaded successfully from environment",
				Loaded:      true,
				Format:      format,
				CharLength:  len(encryptionKey),
				ParsedBytes: len(key),
			})
		}
	}

	if database != nil {
		encryptionKeyService := service.NewEncryptionKeyService(database)
		rotationResult, rotationErr := service.NewEncryptionKeyRotationService(database).RotateOnStartIfConfigured()
		if rotationErr != nil {
			log.Fatalf("[SECURITY] FATAL: encryption key rotation failed: %v", rotationErr)
		}
		switch rotationResult.Status {
		case "completed":
			logger.Info("[SECURITY] Rotated encrypted configs to the active ENCRYPTION_KEY (%d agents, %d question set overrides)", rotationResult.AgentsRotated, rotationResult.QuestionSetAgentsRotated)
		case "lock_busy":
			logger.Warn("[SECURITY] Encryption key rotation is already running in another instance; this revision will continue with dual-key reads")
		case "skipped_same_key":
			logger.Info("[SECURITY] Encryption key rotation requested, but ENCRYPTION_KEY and ENCRYPTION_KEY_PREVIOUS resolve to the same key")
		}

		health, err := encryptionKeyService.ReconcileCurrentKey()
		if err != nil {
			logger.Warn("[SECURITY] Failed to reconcile encryption key state: %v", err)
		} else if health.StateStatus == "mismatch" {
			autoPromote := parseEncryptionAutoPromoteFlag(os.Getenv("ENCRYPTION_KEY_AUTO_PROMOTE"))
			if blocked, reason := shouldBlockStartupForEncryptionHealth(os.Getenv("APP_ENV"), health, autoPromote); blocked {
				log.Fatalf("[SECURITY] FATAL: %s", reason)
			}
			// Auto-promote is either requested or we are not in production.
			// Proceed but emit a loud warning so operators are aware data loss
			// may have occurred for agents encrypted with the previous key.
			current, observeErr := encryptionKeyService.ObserveCurrentKey()
			if observeErr != nil {
				logger.Warn("[SECURITY] Key mismatch detected but cannot read current key for auto-promote: %v", observeErr)
			} else if promoteErr := encryptionKeyService.PromoteCurrentKeyState(current); promoteErr != nil {
				logger.Warn("[SECURITY] Key mismatch auto-promote failed: %v", promoteErr)
			} else {
				logger.Warn("[SECURITY] ENCRYPTION_KEY fingerprint mismatch — current key auto-promoted as active. Agents encrypted with the previous key will require re-entering credentials.")
				encryptionKeyService.AppendKeyStateHistory(
					"auto_promoted",
					health.StoredFingerprintPrefix,
					health.ObservedFingerprintPrefix,
					"mismatch",
					"match",
					"startup_auto_promote",
					"",
				)
			}
		} else if blocked, reason := shouldBlockStartupForEncryptionHealth(os.Getenv("APP_ENV"), health, false); blocked {
			log.Fatalf("[SECURITY] FATAL: %s. The active ENCRYPTION_KEY cannot verify the stored sentinel — possible key corruption or DB tampering. Use ENCRYPTION_KEY_PREVIOUS + ENCRYPTION_KEY_ROTATE_ON_START for proper key rotation.", reason)
		} else {
			logger.Info("[SECURITY] %s", health.StateSummary)
		}
	}

	engine := orchestrator.NewEngine(database, workerCount, queueSize)
	if database != nil {
		res := database.Model(&models.Run{}).Where("status = ?", "running").Update("status", "cancelled")
		if res.Error != nil {
			logger.Error("[RUN] Failed to cancel stale runs on startup: %v", res.Error)
		} else if res.RowsAffected > 0 {
			logger.Info("[RUN] Marked %d stale running run(s) as cancelled on startup", res.RowsAffected)
		}
	}

	// Initialize Firebase Admin SDK
	fbClient, err := firebase.InitFirebase()
	if err != nil {
		logger.Error("[FIREBASE] Failed to initialize Firebase: %v", err)
	}

	// Initialize WebSocket hub
	hub := api.NewHub(database, engine, jwtSecret, fbClient)
	go hub.Run()

	// Route orchestrator events through the question-set audience so active
	// collaborators receive task/run updates in real time. Falls back to the
	// owner's workspace if the run can't be resolved (e.g. non-run events or
	// corrupted correlation id).
	engine.SetEventCallback(func(workspaceID uuid.UUID, eventType string, correlationID string, payload any) {
		if runID, err := uuid.Parse(correlationID); err == nil && runID != uuid.Nil {
			if sendErr := hub.SendEventForRun(runID, eventType, correlationID, payload); sendErr == nil {
				return
			}
		}
		hub.SendEvent(workspaceID, eventType, correlationID, payload)
	})
	engine.Start()

	// Periodic heartbeat — always visible in every environment so operators
	// can confirm the service is alive.  Reports goroutine count and memory
	// so basic health trends are observable in Cloud Run Logs.
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			logger.Health("System OK — goroutines=%d heap_alloc=%dMB heap_sys=%dMB",
				runtime.NumGoroutine(),
				mem.HeapAlloc/1024/1024,
				mem.HeapSys/1024/1024,
			)
		}
	}()

	// Initialize handlers with WebSocket Hub support

	// Secret is now stable across restarts
	logger.Info("[AUTH] System session secret initialized (all previous sessions invalidated)")
	authHandler := handlers.NewAuthHandler(database, jwtSecret)

	// ========== REST API Routes ==========

	// Public Routes
	// Public Routes with Rate Limiting (20 requests per minute)
	authRateLimiter := echoMiddleware.RateLimiter(echoMiddleware.NewRateLimiterMemoryStore(20))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"status":   "ok",
			"revision": appRevision,
		})
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
		logger.Debug("[WS] Upgrade attempt host=%q origin=%q x-forwarded-host=%q x-forwarded-proto=%q subprotocol-header=%q workspace_id=%q",
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
			// Parse token — enforce HMAC signing to prevent algorithm-confusion attacks.
			token, err := jwt.ParseWithClaims(tokenString, &middleware.Claims{}, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(jwtSecret), nil
			})

			if err == nil && token.Valid {
				claims, _ := token.Claims.(*middleware.Claims)
				// Strict parse: a token with a malformed UserID claim must not
				// be treated as authenticated. OrgID is optional (users can be
				// logged in without an organization context).
				parsedUserID, userErr := uuid.Parse(claims.UserID)
				if userErr != nil {
					logger.Warn("[WS] Token has invalid UserID claim %q: %v", claims.UserID, userErr)
				} else {
					userID = parsedUserID
					isAuthenticated = true
					if claims.OrgID != "" {
						if parsedOrgID, orgErr := uuid.Parse(claims.OrgID); orgErr == nil {
							orgID = parsedOrgID
						} else {
							logger.Warn("[WS] Token has invalid OrgID claim %q: %v", claims.OrgID, orgErr)
						}
					}
				}
			}
		}

		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			logger.Error("[WS] Upgrade failed host=%q origin=%q x-forwarded-host=%q err=%v",
				req.Host,
				origin,
				forwardedHost,
				err,
			)
			return err
		}
		logger.Debug("[WS] Subprotocol selected: %q", ws.Subprotocol())

		workspaceIDStr := c.QueryParam("workspace_id")
		if workspaceIDStr != "" {
			if parsed, perr := uuid.Parse(workspaceIDStr); perr == nil {
				workspaceID = parsed
			} else {
				logger.Warn("[WS] Ignoring invalid workspace_id query param %q: %v", workspaceIDStr, perr)
			}
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
	logger.Info("=======================================================")
	logger.Info("BENCHMARKING PLATFORM - Go Server")
	logger.Info("Starting on port %s", port)
	logger.Info("Runner Mode: go (in-process)")
	logger.Info("=======================================================")
	e.Logger.Fatal(e.Start(":" + port))
}
