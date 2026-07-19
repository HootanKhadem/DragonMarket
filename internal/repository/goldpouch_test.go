package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

// createTestGuild is a shared test fixture helper for gold_pouch/listing/
// auction/bid tests that just need "a guild", without an item attached.
func createTestGuild(t *testing.T, ctx context.Context, tx DBTX, name string) Guild {
	t.Helper()
	leader := createTestCharacter(t, ctx, tx, name+" Leader")
	g, err := NewGuildRepository().Create(ctx, tx, Guild{
		Name:              name,
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guild %q: %v", name, err)
	}
	return g
}

func TestGoldPouchRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "GoldPouch Test Guild 1")

	repo := NewGoldPouchRepository()
	createdPouch, err := repo.Create(ctx, tx, GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 5000,
		SpentToday:         0,
		LastResetDate:      time.Now().UTC().Truncate(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdPouch.UsableBalance != 800 {
		t.Errorf("Create().UsableBalance = %d, want 800 (generated, total-reserved)", createdPouch.UsableBalance)
	}

	fetchedPouch, err := repo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if fetchedPouch.TotalBalance != 1000 || fetchedPouch.ReservedBalance != 200 || fetchedPouch.UsableBalance != 800 {
		t.Errorf("GetByGuildID() = %+v, want TotalBalance=1000 ReservedBalance=200 UsableBalance=800", fetchedPouch)
	}
}

func TestGoldPouchRepository_GetByGuildID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewGoldPouchRepository()

	_, err := repo.GetByGuildID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByGuildID() error = %v, want ErrNotFound", err)
	}
}

func TestGoldPouchRepository_GetByGuildIDForUpdate_AndUpdate(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "GoldPouch Test Guild 2")
	repo := NewGoldPouchRepository()

	createdPouch, err := repo.Create(ctx, tx, GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       500,
		ReservedBalance:    0,
		DailySpendingLimit: 2000,
		SpentToday:         0,
		LastResetDate:      time.Now().UTC().Truncate(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// The locking variant: only meaningful inside an explicit transaction,
	// which is exactly what beginTx gives every test.
	locked, err := repo.GetByGuildIDForUpdate(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildIDForUpdate() error = %v", err)
	}
	locked.ReservedBalance = 150
	locked.SpentToday = 50

	if err := repo.Update(ctx, tx, locked); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	fetchedPouch, err := repo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if fetchedPouch.ReservedBalance != 150 || fetchedPouch.SpentToday != 50 {
		t.Errorf("GetByGuildID() after Update() = %+v, want ReservedBalance=150 SpentToday=50", fetchedPouch)
	}
	// usable_balance must still be consistent (generated), never drifted by
	// our Update() (which never touches that column).
	if fetchedPouch.UsableBalance != createdPouch.TotalBalance-150 {
		t.Errorf("GetByGuildID().UsableBalance = %d, want %d", fetchedPouch.UsableBalance, createdPouch.TotalBalance-150)
	}
}
