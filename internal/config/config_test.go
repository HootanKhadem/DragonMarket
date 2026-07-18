package config

import (
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "DATABASE_URL", "ORACLE_REFRESH_INTERVAL", "AUCTION_SWEEP_INTERVAL"} {
		t.Setenv(key, "")
	}
}

func TestLoad_MissingDatabaseURL_ReturnsError(t *testing.T) {
	clearEnv(t)

	_, err := Load()

	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.OracleRefreshInterval != 30*time.Second {
		t.Errorf("OracleRefreshInterval = %v, want %v", cfg.OracleRefreshInterval, 30*time.Second)
	}
	if cfg.AuctionSweepInterval != 10*time.Second {
		t.Errorf("AuctionSweepInterval = %v, want %v", cfg.AuctionSweepInterval, 10*time.Second)
	}
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("PORT", "9090")
	t.Setenv("ORACLE_REFRESH_INTERVAL", "5s")
	t.Setenv("AUCTION_SWEEP_INTERVAL", "1s")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.OracleRefreshInterval != 5*time.Second {
		t.Errorf("OracleRefreshInterval = %v, want %v", cfg.OracleRefreshInterval, 5*time.Second)
	}
	if cfg.AuctionSweepInterval != 1*time.Second {
		t.Errorf("AuctionSweepInterval = %v, want %v", cfg.AuctionSweepInterval, 1*time.Second)
	}
}
