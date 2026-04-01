package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestJWTMiddleware(t *testing.T) {
	e := echo.New()

	secret := "test-secret-key"
	config := JWTConfig{
		SecretKey:   secret,
		SkipPaths:   []string{"/health", "/public"},
		TokenLookup: "header:Authorization",
	}

	middleware := JWTMiddleware(config)

	t.Run("allows skipped paths without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/health")

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects requests without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/protected")

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("accepts valid token", func(t *testing.T) {
		userID := uuid.New().String()
		workspaceID := uuid.New().String()

		token, err := GenerateToken(userID, workspaceID, uuid.New().String(), "test@example.com", secret, "")
		assert.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/protected")

		handler := middleware(func(c echo.Context) error {
			// Verify claims are available
			uid := c.Get(string(UserIDKey))
			wsid := c.Get(string(WorkspaceIDKey))
			assert.Equal(t, userID, uid)
			assert.Equal(t, workspaceID, wsid)
			return c.String(http.StatusOK, "OK")
		})

		err = handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/protected")

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects token with wrong secret", func(t *testing.T) {
		token, _ := GenerateToken(uuid.New().String(), uuid.New().String(), uuid.New().String(), "test@example.com", "wrong-secret", "")

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/protected")

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestWorkspaceScopeMiddleware(t *testing.T) {
	e := echo.New()

	middleware := WorkspaceScopeMiddleware()

	t.Run("allows matching workspace", func(t *testing.T) {
		workspaceID := uuid.New()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(workspaceID.String())
		c.Set(string(WorkspaceIDKey), workspaceID.String())

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects mismatched workspace", func(t *testing.T) {
		tokenWorkspace := uuid.New()
		requestWorkspace := uuid.New()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("workspace_id")
		c.SetParamValues(requestWorkspace.String())
		c.Set(string(WorkspaceIDKey), tokenWorkspace.String())

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestGenerateToken(t *testing.T) {
	secret := "test-secret"
	userID := uuid.New().String()
	workspaceID := uuid.New().String()
	email := "test@example.com"

	tokenString, err := GenerateToken(userID, workspaceID, uuid.New().String(), email, secret, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	// Parse and verify
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims := token.Claims.(*Claims)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, workspaceID, claims.WorkspaceID)
	assert.Equal(t, email, claims.Email)
	assert.NotNil(t, claims.ExpiresAt)
	assert.WithinDuration(t, time.Now().UTC().Add(24*time.Hour), claims.ExpiresAt.Time, 5*time.Second)
}

func TestGenerateTokenWithImpersonation(t *testing.T) {
	secret := "test-secret"
	userID := uuid.New().String()
	workspaceID := uuid.New().String()
	orgID := uuid.New().String()
	email := "test@example.com"
	impersonatorID := uuid.New().String()

	tokenString, err := GenerateToken(userID, workspaceID, orgID, email, secret, impersonatorID)
	assert.NoError(t, err)

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	assert.NoError(t, err)

	claims := token.Claims.(*Claims)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, impersonatorID, claims.ImpersonatorID)
}

func TestGetUserID(t *testing.T) {
	e := echo.New()

	t.Run("extracts valid UUID", func(t *testing.T) {
		userID := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(string(UserIDKey), userID.String())

		result := GetUserID(c)
		assert.Equal(t, userID, result)
	})

	t.Run("returns nil UUID when not set", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		result := GetUserID(c)
		assert.Equal(t, uuid.Nil, result)
	})
}
