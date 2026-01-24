package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Claims represents the JWT claims structure
type Claims struct {
	UserID         string `json:"user_id"`
	WorkspaceID    string `json:"workspace_id"`
	OrgID          string `json:"org_id"`
	Email          string `json:"email"`
	ImpersonatorID string `json:"impersonator_id,omitempty"`
	jwt.RegisteredClaims
}

// ContextKey is the key for storing auth info in context
type ContextKey string

const (
	UserIDKey         ContextKey = "user_id"
	WorkspaceIDKey    ContextKey = "workspace_id"
	OrgIDKey          ContextKey = "org_id"
	ImpersonatorIDKey ContextKey = "impersonator_id"
)

// JWTConfig holds configuration for JWT middleware
type JWTConfig struct {
	SecretKey   string
	SkipPaths   []string
	TokenLookup string // "header:Authorization" or "query:token"
}

// DefaultJWTConfig returns default configuration
func DefaultJWTConfig() JWTConfig {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-in-production"
	}
	return JWTConfig{
		SecretKey:   secret,
		SkipPaths:   []string{"/health", "/ws", "/auth/login", "/auth/register", "/auth/webauthn/login"},
		TokenLookup: "header:Authorization",
	}
}

// JWTMiddleware creates a JWT authentication middleware
func JWTMiddleware(config JWTConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()

			// Skip authentication for certain paths
			for _, skipPath := range config.SkipPaths {
				if strings.HasPrefix(path, skipPath) {
					return next(c)
				}
			}

			// Extract token
			var tokenString string

			// 1. Try Authorization header
			auth := c.Request().Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				tokenString = strings.TrimPrefix(auth, "Bearer ")
			}

			// 2. Try cookie if header is empty
			if tokenString == "" {
				if cookie, err := c.Cookie("token"); err == nil {
					tokenString = cookie.Value
				}
			}

			if tokenString == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "missing authorization token",
				})
			}

			// Parse and validate token
			token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
				return []byte(config.SecretKey), nil
			})

			if err != nil || !token.Valid {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "invalid or expired token",
				})
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "invalid token claims",
				})
			}

			// Store user info in context
			c.Set(string(UserIDKey), claims.UserID)
			c.Set(string(WorkspaceIDKey), claims.WorkspaceID)
			c.Set(string(OrgIDKey), claims.OrgID)
			if claims.ImpersonatorID != "" {
				c.Set(string(ImpersonatorIDKey), claims.ImpersonatorID)
			}

			return next(c)
		}
	}
}

// GetUserID extracts user ID from context
func GetUserID(c echo.Context) uuid.UUID {
	if id, ok := c.Get(string(UserIDKey)).(string); ok {
		if uid, err := uuid.Parse(id); err == nil {
			return uid
		}
	}
	return uuid.Nil
}

// GetWorkspaceID extracts workspace ID from context
func GetWorkspaceID(c echo.Context) uuid.UUID {
	if id, ok := c.Get(string(WorkspaceIDKey)).(string); ok {
		if uid, err := uuid.Parse(id); err == nil {
			return uid
		}
	}
	return uuid.Nil
}

// GetOrgID extracts organization ID from context
func GetOrgID(c echo.Context) uuid.UUID {
	if id, ok := c.Get(string(OrgIDKey)).(string); ok {
		if uid, err := uuid.Parse(id); err == nil {
			return uid
		}
	}
	return uuid.Nil
}

// GetImpersonatorID extracts impersonator ID from context
func GetImpersonatorID(c echo.Context) uuid.UUID {
	if id, ok := c.Get(string(ImpersonatorIDKey)).(string); ok {
		if uid, err := uuid.Parse(id); err == nil {
			return uid
		}
	}
	return uuid.Nil
}

// WorkspaceScopeMiddleware ensures requests are scoped to the user's workspace
func WorkspaceScopeMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get workspace from token
			tokenWorkspaceID := GetWorkspaceID(c)
			if tokenWorkspaceID == uuid.Nil {
				// No workspace in token, allow (for development)
				return next(c)
			}

			// Check if request has workspace_id param and validate it matches
			paramWorkspaceID := c.Param("workspace_id")
			if paramWorkspaceID != "" {
				reqWsID, err := uuid.Parse(paramWorkspaceID)
				if err != nil {
					return c.JSON(http.StatusBadRequest, map[string]string{
						"error": "invalid workspace_id",
					})
				}
				if reqWsID != tokenWorkspaceID {
					return c.JSON(http.StatusForbidden, map[string]string{
						"error": "access denied to this workspace",
					})
				}
			}

			return next(c)
		}
	}
}

// OrganizationScopeMiddleware ensures requests are scoped to the user's organization
func OrganizationScopeMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Basic isolation is handled by handlers, but we can add more here later.
			return next(c)
		}
	}
}

// GenerateToken generates a JWT token
func GenerateToken(userID, workspaceID, orgID, email, secret string, impersonatorID string) (string, error) {
	claims := &Claims{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		OrgID:          orgID,
		Email:          email,
		ImpersonatorID: impersonatorID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
