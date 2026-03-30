package handlers

import (
	"net/http"

	"aegis/internal/modules/reconciliation"

	"github.com/gin-gonic/gin"
)

type ReconciliationHandler struct {
	service *reconciliation.Service
}

func NewReconciliationHandler(service *reconciliation.Service) *ReconciliationHandler {
	return &ReconciliationHandler{service: service}
}

func (h *ReconciliationHandler) Run(c *gin.Context) {
	result, err := h.service.Run(c.Request.Context(), "manual")
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	c.JSON(http.StatusOK, reconciliationRunResponse{
		Data: reconciliationRunData{
			CheckedCount:  result.CheckedCount,
			MismatchCount: result.MismatchCount,
			MatchedCount:  result.MatchedCount,
			TriggerSource: "manual",
		},
	})
}

func (h *ReconciliationHandler) ListMismatches(c *gin.Context) {
	results, err := h.service.ListLatestMismatches(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	data := make([]reconciliationResultResponse, 0, len(results))
	for _, result := range results {
		data = append(data, newReconciliationResultResponse(result))
	}

	c.JSON(http.StatusOK, reconciliationMismatchListEnvelope{Data: data})
}
