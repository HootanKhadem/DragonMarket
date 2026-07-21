package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type ItemService interface {
	CreateItem(ctx context.Context, in service.CreateItemInput) (service.CreateItemResult, error)
	GetItem(ctx context.Context, id int64) (service.ItemView, error)
	ListItems(ctx context.Context, limit, offset int) ([]service.ItemView, error)
	PurchaseItem(ctx context.Context, in service.PurchaseInput) (service.PurchaseResult, error)
}

var _ ItemService = (*service.ItemService)(nil)

type ItemHandlers struct {
	svc ItemService
}

func NewItemHandlers(svc ItemService) *ItemHandlers {
	return &ItemHandlers{svc: svc}
}

// --- request/response DTOs ---

type createItemRequest struct {
	Name              string                `json:"name" binding:"required"`
	LandOfOrigin      string                `json:"land_of_origin" binding:"required"`
	ForgerCharacterID int64                 `json:"forger_character_id" binding:"required"`
	Rarity            repository.ItemRarity `json:"rarity" binding:"required"`
	// Price is a pointer (like Quantity/GuildID) so an omitted field is
	// distinguishable from an explicit price:0 at the service layer, which
	// rejects both for COMMON/RARE items (service.ErrMissingPrice).
	Price    *int   `json:"price"`
	GuildID  *int64 `json:"guild_id"`
	Quantity *int   `json:"quantity"`
}

type itemResponse struct {
	ID                int64                 `json:"id"`
	Name              string                `json:"name"`
	LandOfOrigin      string                `json:"land_of_origin"`
	Rarity            repository.ItemRarity `json:"rarity"`
	ForgerCharacterID int64                 `json:"forger_character_id"`
	Price             int                   `json:"price"`
	IsLegendary       bool                  `json:"is_legendary"`
	// AuctionOnly makes it explicit in the response that legendary items
	// can't be bought via POST /items/{id}/purchase -- only via the
	// auction flow.
	AuctionOnly bool `json:"auction_only"`
}

func newItemResponse(v service.ItemView) itemResponse {
	return itemResponse{
		ID:                v.ID,
		Name:              v.Name,
		LandOfOrigin:      v.LandOfOrigin,
		Rarity:            v.Rarity,
		ForgerCharacterID: v.ForgerCharacterID,
		Price:             v.Price,
		IsLegendary:       v.IsLegendary,
		AuctionOnly:       v.IsLegendary,
	}
}

type createItemResponse struct {
	Item      itemResponse `json:"item"`
	GuildID   int64        `json:"guild_id"`
	Quantity  int          `json:"quantity"`
	ListingID *int64       `json:"listing_id,omitempty"`
}

type purchaseRequest struct {
	Quantity int `json:"quantity" binding:"required"`
}

type purchaseResponse struct {
	ItemID        int64                    `json:"item_id"`
	Quantity      int                      `json:"quantity"`
	UnitPrice     int                      `json:"unit_price"`
	TotalPrice    int                      `json:"total_price"`
	SellerGuildID int64                    `json:"seller_guild_id"`
	ListingID     int64                    `json:"listing_id"`
	ListingStatus repository.ListingStatus `json:"listing_status"`
}

// --- handlers ---

func (h *ItemHandlers) Create(c *gin.Context) {
	var req createItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	result, err := h.svc.CreateItem(c.Request.Context(), service.CreateItemInput{
		Name:              req.Name,
		LandOfOrigin:      req.LandOfOrigin,
		ForgerCharacterID: req.ForgerCharacterID,
		Rarity:            req.Rarity,
		Price:             req.Price,
		GuildID:           req.GuildID,
		Quantity:          req.Quantity,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, createItemResponse{
		Item:      newItemResponse(result.Item),
		GuildID:   result.GuildID,
		Quantity:  result.Quantity,
		ListingID: result.ListingID,
	})
}

func (h *ItemHandlers) List(c *gin.Context) {
	limit, offset := parseLimitOffset(c)

	items, err := h.svc.ListItems(c.Request.Context(), limit, offset)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	out := make([]itemResponse, len(items))
	for i, it := range items {
		out[i] = newItemResponse(it)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *ItemHandlers) Get(c *gin.Context) {
	id, err := parseItemID(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ITEM_ID", err.Error())
		return
	}

	view, err := h.svc.GetItem(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newItemResponse(view))
}

func (h *ItemHandlers) Purchase(c *gin.Context) {
	itemID, err := parseItemID(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ITEM_ID", err.Error())
		return
	}

	guildID := guildIDFromContext(c)

	var req purchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	result, err := h.svc.PurchaseItem(c.Request.Context(), service.PurchaseInput{
		ItemID:       itemID,
		BuyerGuildID: guildID,
		Quantity:     req.Quantity,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, purchaseResponse{
		ItemID:        result.ItemID,
		Quantity:      result.Quantity,
		UnitPrice:     result.UnitPrice,
		TotalPrice:    result.TotalPrice,
		SellerGuildID: result.SellerGuildID,
		ListingID:     result.ListingID,
		ListingStatus: result.ListingStatus,
	})
}

func parseItemID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, errors.New("item id must be an integer")
	}
	return id, nil
}

func parseLimitOffset(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	return limit, offset
}
