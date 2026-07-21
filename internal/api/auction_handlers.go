package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type AuctionService interface {
	CreateAuction(ctx context.Context, in service.CreateAuctionInput) (service.AuctionView, error)
	GetAuction(ctx context.Context, id int64) (service.AuctionView, error)
	ListActiveAuctions(ctx context.Context, limit, offset int) ([]service.AuctionView, error)
	PlaceBid(ctx context.Context, in service.PlaceBidInput) (service.BidView, error)
	CancelBid(ctx context.Context, in service.CancelBidInput) error
}

var _ AuctionService = (*service.AuctionService)(nil)

type AuctionHandlers struct {
	svc AuctionService
}

func NewAuctionHandlers(svc AuctionService) *AuctionHandlers {
	return &AuctionHandlers{svc: svc}
}

// --- request/response DTOs ---

type createAuctionRequest struct {
	ItemID          int64 `json:"item_id" binding:"required"`
	DurationSeconds int   `json:"duration_seconds" binding:"required"`
}

type auctionResponse struct {
	ID           int64                    `json:"id"`
	ItemID       int64                    `json:"item_id"`
	OwnerGuildID int64                    `json:"owner_guild_id"`
	Status       repository.AuctionStatus `json:"status"`
	StartTime    time.Time                `json:"start_time"`
	EndTime      time.Time                `json:"end_time"`
	BasePrice    int                      `json:"base_price"`
}

func newAuctionResponse(v service.AuctionView) auctionResponse {
	return auctionResponse{
		ID:           v.ID,
		ItemID:       v.ItemID,
		OwnerGuildID: v.OwnerGuildID,
		Status:       v.Status,
		StartTime:    v.StartTime,
		EndTime:      v.EndTime,
		BasePrice:    v.BasePrice,
	}
}

type placeBidRequest struct {
	Amount int `json:"amount" binding:"required"`
}

type bidResponse struct {
	ID        int64                `json:"id"`
	AuctionID int64                `json:"auction_id"`
	GuildID   int64                `json:"guild_id"`
	Amount    int                  `json:"amount"`
	Status    repository.BidStatus `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
}

func newBidResponse(v service.BidView) bidResponse {
	return bidResponse{
		ID:        v.ID,
		AuctionID: v.AuctionID,
		GuildID:   v.GuildID,
		Amount:    v.Amount,
		Status:    v.Status,
		CreatedAt: v.CreatedAt,
	}
}

// --- handlers ---

func (h *AuctionHandlers) Create(c *gin.Context) {
	guildID, err := guildIDFromHeader(c)
	switch {
	case errors.Is(err, errMissingGuildHeader):
		writeError(c, http.StatusBadRequest, "MISSING_GUILD_HEADER", err.Error())
		return
	case errors.Is(err, errInvalidGuildHeader):
		writeError(c, http.StatusBadRequest, "INVALID_GUILD_HEADER", err.Error())
		return
	}

	var req createAuctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	result, err := h.svc.CreateAuction(c.Request.Context(), service.CreateAuctionInput{
		ItemID:          req.ItemID,
		OwnerGuildID:    guildID,
		DurationSeconds: req.DurationSeconds,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, newAuctionResponse(result))
}

func (h *AuctionHandlers) List(c *gin.Context) {
	limit, offset := parseLimitOffset(c)

	auctions, err := h.svc.ListActiveAuctions(c.Request.Context(), limit, offset)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	out := make([]auctionResponse, len(auctions))
	for i, a := range auctions {
		out[i] = newAuctionResponse(a)
	}
	c.JSON(http.StatusOK, gin.H{"auctions": out})
}

func (h *AuctionHandlers) Get(c *gin.Context) {
	id, err := parseItemID(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_AUCTION_ID", err.Error())
		return
	}

	view, err := h.svc.GetAuction(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newAuctionResponse(view))
}

func (h *AuctionHandlers) PlaceBid(c *gin.Context) {
	itemID, err := parseItemID(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ITEM_ID", err.Error())
		return
	}

	guildID, err := guildIDFromHeader(c)
	switch {
	case errors.Is(err, errMissingGuildHeader):
		writeError(c, http.StatusBadRequest, "MISSING_GUILD_HEADER", err.Error())
		return
	case errors.Is(err, errInvalidGuildHeader):
		writeError(c, http.StatusBadRequest, "INVALID_GUILD_HEADER", err.Error())
		return
	}

	var req placeBidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	bid, err := h.svc.PlaceBid(c.Request.Context(), service.PlaceBidInput{
		ItemID:  itemID,
		GuildID: guildID,
		Amount:  req.Amount,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, newBidResponse(bid))
}

func (h *AuctionHandlers) CancelBid(c *gin.Context) {
	itemID, err := parseItemID(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ITEM_ID", err.Error())
		return
	}

	bidID, err := parseInt64Param(c, "bid_id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BID_ID", err.Error())
		return
	}

	guildID, err := guildIDFromHeader(c)
	switch {
	case errors.Is(err, errMissingGuildHeader):
		writeError(c, http.StatusBadRequest, "MISSING_GUILD_HEADER", err.Error())
		return
	case errors.Is(err, errInvalidGuildHeader):
		writeError(c, http.StatusBadRequest, "INVALID_GUILD_HEADER", err.Error())
		return
	}

	if err := h.svc.CancelBid(c.Request.Context(), service.CancelBidInput{
		ItemID:  itemID,
		BidID:   bidID,
		GuildID: guildID,
	}); err != nil {
		writeServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func parseInt64Param(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return id, nil
}
