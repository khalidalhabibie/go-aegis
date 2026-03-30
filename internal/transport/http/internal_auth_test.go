package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func TestInternalAuthMiddlewareRejectsMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(internalAuthMiddleware(config.InternalAuthConfig{
		HeaderName: "X-Test-Key",
		APIKey:     "secret",
	}, zerolog.Nop()))
	router.GET("/ops", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/ops", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestInternalAuthMiddlewareAllowsValidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(internalAuthMiddleware(config.InternalAuthConfig{
		HeaderName: "X-Test-Key",
		APIKey:     "secret",
	}, zerolog.Nop()))
	router.GET("/ops", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/ops", nil)
	request.Header.Set("X-Test-Key", "secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestInternalAuthMiddlewareRejectsUnconfiguredProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(internalAuthMiddleware(config.InternalAuthConfig{
		HeaderName: "X-Test-Key",
	}, zerolog.Nop()))
	router.GET("/ops", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/ops", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
}
