package handlers

import (
	"errors"
	"net/http"

	"aegis/internal/modules/wallets"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	service *wallets.Service
}

func NewWalletHandler(service *wallets.Service) *WalletHandler {
	return &WalletHandler{service: service}
}

func (h *WalletHandler) Create(c *gin.Context) {
	var request createWalletRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}

	wallet, err := h.service.CreateWallet(c.Request.Context(), wallets.CreateInput{
		Chain:       request.Chain,
		Address:     request.Address,
		Label:       request.Label,
		SigningType: request.SigningType,
		Status:      request.Status,
	})
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, walletEnvelope{Data: newWalletResponse(wallet)})
}

func (h *WalletHandler) List(c *gin.Context) {
	walletsList, err := h.service.ListWallets(c.Request.Context())
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	data := make([]walletResponse, 0, len(walletsList))
	for _, wallet := range walletsList {
		data = append(data, newWalletResponse(wallet))
	}

	c.JSON(http.StatusOK, walletListEnvelope{Data: data})
}

func (h *WalletHandler) GetByID(c *gin.Context) {
	wallet, err := h.service.GetWallet(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, walletEnvelope{Data: newWalletResponse(wallet)})
}

func (h *WalletHandler) handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, wallets.ErrValidation):
		writeError(c, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, wallets.ErrDuplicateActiveWallet):
		writeError(c, http.StatusConflict, "wallet_conflict", "an active wallet with this chain and address already exists")
	case errors.Is(err, wallets.ErrWalletNotFound):
		writeError(c, http.StatusNotFound, "wallet_not_found", "wallet not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
