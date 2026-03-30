package http

import (
	"net/http"
	"strings"
	"time"

	"aegis/internal/config"
	"aegis/internal/transport/http/handlers"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func NewServer(
	cfg config.HTTPConfig,
	internalAuthCfg config.InternalAuthConfig,
	environment string,
	log zerolog.Logger,
	healthHandler *handlers.HealthHandler,
	transferHandler *handlers.TransferHandler,
	walletHandler *handlers.WalletHandler,
	reconciliationHandler *handlers.ReconciliationHandler,
) *http.Server {
	gin.SetMode(resolveGinMode(environment))

	router := gin.New()
	router.Use(requestLogger(log), gin.Recovery())

	router.GET("/healthz", healthHandler.Get)

	apiV1 := router.Group("/api/v1")
	apiV1.POST("/transfers", transferHandler.Create)
	apiV1.GET("/transfers/:id", transferHandler.GetByID)
	apiV1.GET("/transfers", transferHandler.List)

	internalOps := apiV1.Group("")
	internalOps.Use(internalAuthMiddleware(internalAuthCfg, log))
	internalOps.POST("/wallets", walletHandler.Create)
	internalOps.GET("/wallets", walletHandler.List)
	internalOps.GET("/wallets/:id", walletHandler.GetByID)
	internalOps.POST("/jobs/reconcile", reconciliationHandler.Run)
	internalOps.GET("/reconciliation/mismatches", reconciliationHandler.ListMismatches)

	return &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func resolveGinMode(environment string) string {
	switch strings.ToLower(environment) {
	case "production", "prod":
		return gin.ReleaseMode
	case "test":
		return gin.TestMode
	default:
		return gin.DebugMode
	}
}

func requestLogger(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		event := log.Info()
		status := c.Writer.Status()

		switch {
		case status >= http.StatusInternalServerError:
			event = log.Error()
		case status >= http.StatusBadRequest:
			event = log.Warn()
		}

		event.
			Int("status", status).
			Str("method", c.Request.Method).
			Str("path", c.FullPath()).
			Str("client_ip", c.ClientIP()).
			Dur("latency", time.Since(startedAt)).
			Msg("http_request")
	}
}
