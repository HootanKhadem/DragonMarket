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
	Pool        *pgxpool.Pool
	Items       repository.ItemRepository
	Listings    repository.ListingRepository
	Inventories repository.InventoryRepository
	Guilds      repository.GuildRepository
	GoldPouches *service.GoldPouchService
	PriceCache  *oracle.Cache
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

	return router
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
