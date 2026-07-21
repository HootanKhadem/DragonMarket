package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"DragonMarket/internal/repository"
)

func newTestWalletService(db repository.DBTX) *WalletService {
	return NewWalletService(db, repository.NewGuildRepository(), repository.NewGoldPouchRepository())
}

func TestWalletService_GetWallet_UnknownGuildID_ReturnsErrGuildNotFound(t *testing.T) {
	ctx := context.Background()
	svc := newTestWalletService(testPool)

	_, err := svc.GetWallet(ctx, 999999999)
	if !errors.Is(err, ErrGuildNotFound) {
		t.Fatalf("GetWallet() error = %v, want ErrGuildNotFound", err)
	}
}

func TestWalletService_GetWallet_HappyPath_ReturnsPouchFields(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Wallet HappyPath Guild")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 500,
		SpentToday:         150,
		LastResetDate:      today,
	})
	svc := newTestWalletService(tx)

	view, err := svc.GetWallet(ctx, guild.ID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if view.TotalBalance != 1000 || view.ReservedBalance != 200 || view.UsableBalance != 800 ||
		view.DailySpendingLimit != 500 || view.SpentToday != 150 {
		t.Fatalf("GetWallet() = %+v, want TotalBalance=1000 ReservedBalance=200 UsableBalance=800 "+
			"DailySpendingLimit=500 SpentToday=150", view)
	}
}

func TestWalletService_GetWallet_StaleReset_DisplaysSpentTodayAsZeroWithoutWriting(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Wallet StaleReset Guild")
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 500,
		SpentToday:         90,
		LastResetDate:      yesterday,
	})
	svc := newTestWalletService(tx)

	view, err := svc.GetWallet(ctx, guild.ID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if view.SpentToday != 0 {
		t.Errorf("GetWallet().SpentToday = %d, want 0 (stale reset should display as 0)", view.SpentToday)
	}
	if view.TotalBalance != 1000 || view.ReservedBalance != 200 || view.DailySpendingLimit != 500 {
		t.Errorf("GetWallet() = %+v, want TotalBalance=1000 ReservedBalance=200 DailySpendingLimit=500", view)
	}

	raw, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if raw.SpentToday != 90 {
		t.Errorf("underlying row SpentToday = %d, want 90 (GET must not write back)", raw.SpentToday)
	}
	if !raw.LastResetDate.UTC().Equal(yesterday) {
		t.Errorf("underlying row LastResetDate = %v, want %v (GET must not write back)", raw.LastResetDate.UTC(), yesterday)
	}
}

func TestWalletService_GetWallet_SameDayResetDate_DisplaysActualSpentToday(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Wallet SameDay Guild")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		DailySpendingLimit: 500,
		SpentToday:         25,
		LastResetDate:      today,
	})
	svc := newTestWalletService(tx)

	view, err := svc.GetWallet(ctx, guild.ID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if view.SpentToday != 25 {
		t.Errorf("GetWallet().SpentToday = %d, want 25 (not stale, should reflect actual value)", view.SpentToday)
	}
}
