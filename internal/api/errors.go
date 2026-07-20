package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRarity):
		writeError(c, http.StatusBadRequest, "INVALID_RARITY", err.Error())
	case errors.Is(err, service.ErrInvalidPrice):
		writeError(c, http.StatusBadRequest, "INVALID_PRICE", err.Error())
	case errors.Is(err, service.ErrMissingPrice):
		writeError(c, http.StatusBadRequest, "MISSING_PRICE", err.Error())
	case errors.Is(err, service.ErrMissingQuantity):
		writeError(c, http.StatusBadRequest, "MISSING_QUANTITY", err.Error())
	case errors.Is(err, service.ErrInvalidQuantity):
		writeError(c, http.StatusBadRequest, "INVALID_QUANTITY", err.Error())
	case errors.Is(err, service.ErrInvalidAmount):
		writeError(c, http.StatusBadRequest, "INVALID_AMOUNT", err.Error())
	case errors.Is(err, service.ErrGuildNotFound):
		writeError(c, http.StatusNotFound, "GUILD_NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrItemNotFound):
		writeError(c, http.StatusNotFound, "ITEM_NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrNotFound):
		writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrListingNotActive):
		writeError(c, http.StatusConflict, "LISTING_NOT_ACTIVE", err.Error())
	case errors.Is(err, service.ErrInsufficientQuantity):
		writeError(c, http.StatusConflict, "INSUFFICIENT_QUANTITY", err.Error())
	case errors.Is(err, service.ErrLegendaryConflict):
		writeError(c, http.StatusConflict, "LEGENDARY_CONFLICT", err.Error())
	case errors.Is(err, service.ErrInsufficientBalance):
		writeError(c, http.StatusConflict, "INSUFFICIENT_BALANCE", err.Error())
	case errors.Is(err, service.ErrInsufficientReserved):
		writeError(c, http.StatusConflict, "INSUFFICIENT_RESERVED", err.Error())
	case errors.Is(err, service.ErrDailyLimitExceeded):
		writeError(c, http.StatusConflict, "DAILY_LIMIT_EXCEEDED", err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
