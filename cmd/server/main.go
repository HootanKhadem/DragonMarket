package main

import (
	"log/slog"
	"os"

	"DragonMarket/internal/api"
	"DragonMarket/internal/config"
	"DragonMarket/internal/migrate"
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

	router := api.NewRouter()

	logger.Info("starting server", "port", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
