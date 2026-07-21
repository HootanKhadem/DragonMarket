package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/service"
)

type WalletService interface {
	GetWallet(ctx context.Context, guildID int64) (service.WalletView, error)
}

var _ WalletService = (*service.WalletService)(nil)

type WalletHandlers struct {
	svc WalletService
}

func NewWalletHandlers(svc WalletService) *WalletHandlers {
	return &WalletHandlers{svc: svc}
}

// --- request/response DTOs ---

type walletResponse struct {
	TotalBalance       int `json:"total_balance"`
	ReservedBalance    int `json:"reserved_balance"`
	UsableBalance      int `json:"usable_balance"`
	DailySpendingLimit int `json:"daily_spending_limit"`
	SpentToday         int `json:"spent_today"`
}

func newWalletResponse(v service.WalletView) walletResponse {
	return walletResponse{
		TotalBalance:       v.TotalBalance,
		ReservedBalance:    v.ReservedBalance,
		UsableBalance:      v.UsableBalance,
		DailySpendingLimit: v.DailySpendingLimit,
		SpentToday:         v.SpentToday,
	}
}

// --- handlers ---

func (h *WalletHandlers) Get(c *gin.Context) {
	guildID, err := parseInt64Param(c, "id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_GUILD_ID", err.Error())
		return
	}

	view, err := h.svc.GetWallet(c.Request.Context(), guildID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newWalletResponse(view))
}
