package config

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	OracleRefreshInterval time.Duration
	AuctionSweepInterval  time.Duration
}

func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	oracleInterval, err := durationEnv("ORACLE_REFRESH_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	sweepInterval, err := durationEnv("AUCTION_SWEEP_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                  stringEnv("PORT", "8080"),
		DatabaseURL:           databaseURL,
		OracleRefreshInterval: oracleInterval,
		AuctionSweepInterval:  sweepInterval,
	}, nil
}

func stringEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return time.ParseDuration(v)
}
