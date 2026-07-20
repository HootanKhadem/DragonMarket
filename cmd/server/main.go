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

	goldPouches := service.NewGoldPouchService(
		repository.NewGoldPouchRepository(),
		repository.NewTransactionLogRepository(),
	)

	router := api.NewRouter(api.Dependencies{
		Pool:        pool,
		Items:       repository.NewItemRepository(),
		Listings:    repository.NewListingRepository(),
		Inventories: repository.NewInventoryRepository(),
		Guilds:      repository.NewGuildRepository(),
		GoldPouches: goldPouches,
		PriceCache:  priceCache,
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
