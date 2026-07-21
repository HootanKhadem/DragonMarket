package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type Dependencies struct {
	Pool          *pgxpool.Pool
	Items         repository.ItemRepository
	Listings      repository.ListingRepository
	Inventories   repository.InventoryRepository
	Guilds        repository.GuildRepository
	Auctions      repository.AuctionRepository
	Bids          repository.BidRepository
	GoldPouches   *service.GoldPouchService
	GoldPouchRepo repository.GoldPouchRepository
	PriceCache    *oracle.Cache
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/health", healthHandler)

	itemSvc := service.NewItemService(
		deps.Pool, deps.Items, deps.Listings, deps.Inventories, deps.Guilds, deps.GoldPouches, deps.PriceCache,
	)
	itemHandlers := NewItemHandlers(itemSvc)

	router.POST("/items", itemHandlers.Create)
	router.GET("/items", itemHandlers.List)
	router.GET("/items/:id", itemHandlers.Get)
	router.POST("/items/:id/purchase", itemHandlers.Purchase)

	auctionSvc := service.NewAuctionService(
		deps.Pool, deps.Auctions, deps.Bids, deps.Items, deps.Inventories, deps.GoldPouches, deps.PriceCache,
	)
	auctionHandlers := NewAuctionHandlers(auctionSvc)

	router.POST("/auctions", auctionHandlers.Create)
	router.GET("/auctions", auctionHandlers.List)
	router.GET("/auctions/:id", auctionHandlers.Get)
	router.POST("/items/:id/bid", auctionHandlers.PlaceBid)
	router.DELETE("/items/:id/bid/:bid_id", auctionHandlers.CancelBid)

	walletSvc := service.NewWalletService(deps.Pool, deps.Guilds, deps.GoldPouchRepo)
	walletHandlers := NewWalletHandlers(walletSvc)

	router.GET("/guilds/:id/wallet", walletHandlers.Get)

	return router
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
