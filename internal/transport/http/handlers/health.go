package handlers

import (
	"net/http"

	"aegis/internal/modules/health"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	service *health.Service
}

func NewHealthHandler(service *health.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

func (h *HealthHandler) Get(c *gin.Context) {
	report := h.service.Check(c.Request.Context())

	statusCode := http.StatusOK
	if report.Status != "ok" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, report)
}
