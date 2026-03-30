package http

import (
	"crypto/subtle"
	"net/http"

	"aegis/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func internalAuthMiddleware(cfg config.InternalAuthConfig, log zerolog.Logger) gin.HandlerFunc {
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-Aegis-Internal-Key"
	}

	return func(c *gin.Context) {
		if cfg.APIKey == "" {
			log.Error().
				Str("path", c.FullPath()).
				Str("method", c.Request.Method).
				Msg("internal auth api key is not configured")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"code":    "service_unavailable",
					"message": "internal authentication is not configured",
				},
			})
			return
		}

		providedKey := c.GetHeader(headerName)
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(cfg.APIKey)) != 1 {
			log.Warn().
				Str("path", c.FullPath()).
				Str("method", c.Request.Method).
				Str("client_ip", c.ClientIP()).
				Msg("internal auth rejected request")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "unauthorized",
					"message": "invalid internal api key",
				},
			})
			return
		}

		c.Next()
	}
}
