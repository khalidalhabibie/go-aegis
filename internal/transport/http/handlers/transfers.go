package handlers

import (
	"errors"
	"net/http"

	"aegis/internal/modules/transfers"

	"github.com/gin-gonic/gin"
)

type TransferHandler struct {
	service *transfers.Service
}

func NewTransferHandler(service *transfers.Service) *TransferHandler {
	return &TransferHandler{service: service}
}

func (h *TransferHandler) Create(c *gin.Context) {
	var request createTransferRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}

	transfer, created, err := h.service.CreateTransfer(c.Request.Context(), transfers.CreateInput{
		IdempotencyKey:     request.IdempotencyKey,
		Chain:              request.Chain,
		AssetType:          request.AssetType,
		SourceWalletID:     request.SourceWalletID,
		DestinationAddress: request.DestinationAddress,
		Amount:             request.Amount,
		CallbackURL:        request.CallbackURL,
		MetadataJSON:       request.MetadataJSON,
	})
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}

	c.JSON(statusCode, transferEnvelope{Data: newTransferResponse(transfer)})
}

func (h *TransferHandler) GetByID(c *gin.Context) {
	transfer, err := h.service.GetTransfer(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, transferEnvelope{Data: newTransferResponse(transfer)})
}

func (h *TransferHandler) List(c *gin.Context) {
	var query listTransfersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query", "query parameters are invalid")
		return
	}

	result, err := h.service.ListTransfers(c.Request.Context(), transfers.ListInput{
		Limit:  query.Limit,
		Offset: query.Offset,
	})
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	data := make([]transferResponse, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, newTransferResponse(item))
	}

	c.JSON(http.StatusOK, transferListEnvelope{
		Data: data,
		Meta: listMeta{
			Limit:  result.Limit,
			Offset: result.Offset,
			Count:  len(result.Items),
		},
	})
}

func (h *TransferHandler) handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, transfers.ErrValidation):
		writeError(c, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, transfers.ErrTransferNotFound):
		writeError(c, http.StatusNotFound, "transfer_not_found", "transfer not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
