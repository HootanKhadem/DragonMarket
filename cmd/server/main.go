package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"DragonMarket/internal/api"
	"DragonMarket/internal/config"
	"DragonMarket/internal/migrate"
	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
	"DragonMarket/internal/settlement"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := migrate.Up(cfg.DatabaseURL); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	priceCache := startPriceOracleTicker(ctx, cfg, pool, logger)

	goldPouchRepo := repository.NewGoldPouchRepository()
	goldPouches := service.NewGoldPouchService(
		goldPouchRepo,
		repository.NewTransactionLogRepository(),
	)

	startAuctionSettlementTicker(ctx, cfg, pool, goldPouches, logger)

	router := api.NewRouter(api.Dependencies{
		Pool:          pool,
		Items:         repository.NewItemRepository(),
		Listings:      repository.NewListingRepository(),
		Inventories:   repository.NewInventoryRepository(),
		Guilds:        repository.NewGuildRepository(),
		Auctions:      repository.NewAuctionRepository(),
		Bids:          repository.NewBidRepository(),
		GoldPouches:   goldPouches,
		GoldPouchRepo: goldPouchRepo,
		PriceCache:    priceCache,
	})

	logger.Info("starting server", "port", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func startPriceOracleTicker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) *oracle.Cache {
	priceOracle := oracle.NewMockPriceOracleService()
	priceOracle.SetDefault(oracle.MockResponse{Price: 8000, Jitter: 1000})

	priceCache := oracle.NewCache()
	items := repository.NewItemRepository()

	updater := oracle.NewUpdater(priceOracle, priceCache, items, pool, cfg.OracleRefreshInterval)
	updater.Logger = logger

	go updater.Run(ctx)
	logger.Info("price oracle ticker started", "interval", cfg.OracleRefreshInterval)

	return priceCache
}

func startAuctionSettlementTicker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, goldPouches *service.GoldPouchService, logger *slog.Logger) {
	settler := settlement.NewSettler(
		pool,
		repository.NewAuctionRepository(),
		repository.NewBidRepository(),
		repository.NewInventoryRepository(),
		goldPouches,
		cfg.AuctionSweepInterval,
	)
	settler.Logger = logger

	go settler.Run(ctx)
	logger.Info("auction settlement ticker started", "interval", cfg.AuctionSweepInterval)
}
