package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createTestLegendaryItem(t *testing.T, ctx context.Context, tx DBTX, name string) (Item, Guild) {
	t.Helper()
	return createTestItem(t, ctx, tx, name, RarityLegendary, 9000)
}

func TestAuctionRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestLegendaryItem(t, ctx, tx, "Auction Test Item 1")

	repo := NewAuctionRepository()
	start := time.Now().UTC().Truncate(time.Microsecond)
	end := start.Add(time.Hour)

	createdAuction, err := repo.Create(ctx, tx, Auction{
		ItemID:       item.ID,
		OwnerGuildID: guild.ID,
		Status:       AuctionActive,
		StartTime:    start,
		EndTime:      end,
		BasePrice:    9000,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdAuction.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}
	// item_rarity is trigger-populated and constrained to LEGENDARY; the
	// repository must never set it itself.
	if createdAuction.ItemRarity != RarityLegendary {
		t.Errorf("Create().ItemRarity = %q, want LEGENDARY (trigger-populated)", createdAuction.ItemRarity)
	}

	foundAuction, err := repo.GetByID(ctx, tx, createdAuction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if foundAuction.Status != AuctionActive || foundAuction.ItemRarity != RarityLegendary {
		t.Errorf("GetByID() = %+v, want Status=ACTIVE ItemRarity=LEGENDARY", foundAuction)
	}
	if !foundAuction.EndTime.Equal(end) {
		t.Errorf("GetByID().EndTime = %v, want %v", foundAuction.EndTime, end)
	}
}

func TestAuctionRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewAuctionRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestAuctionRepository_GetActiveByItemID(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestLegendaryItem(t, ctx, tx, "Auction Test Item 2")
	repo := NewAuctionRepository()

	start := time.Now().UTC()
	createdAuction, err := repo.Create(ctx, tx, Auction{
		ItemID: item.ID, OwnerGuildID: guild.ID, Status: AuctionActive,
		StartTime: start, EndTime: start.Add(time.Hour), BasePrice: 9000,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	foundAuction, err := repo.GetActiveByItemID(ctx, tx, item.ID)
	if err != nil {
		t.Fatalf("GetActiveByItemID() error = %v", err)
	}
	if foundAuction.ID != createdAuction.ID {
		t.Errorf("GetActiveByItemID().ID = %d, want %d", foundAuction.ID, createdAuction.ID)
	}
}

func TestAuctionRepository_GetByIDForUpdate_AndUpdate(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestLegendaryItem(t, ctx, tx, "Auction Test Item 3")
	repo := NewAuctionRepository()

	start := time.Now().UTC()
	createdAuction, err := repo.Create(ctx, tx, Auction{
		ItemID: item.ID, OwnerGuildID: guild.ID, Status: AuctionActive,
		StartTime: start, EndTime: start.Add(time.Hour), BasePrice: 9000,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	locked, err := repo.GetByIDForUpdate(ctx, tx, createdAuction.ID)
	if err != nil {
		t.Fatalf("GetByIDForUpdate() error = %v", err)
	}
	locked.Status = AuctionExpired
	newEnd := locked.EndTime.Add(5 * time.Minute)
	locked.EndTime = newEnd

	if err := repo.Update(ctx, tx, locked); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	foundAuction, err := repo.GetByID(ctx, tx, createdAuction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if foundAuction.Status != AuctionExpired || !foundAuction.EndTime.Equal(newEnd) {
		t.Errorf("GetByID() after Update() = %+v, want Status=EXPIRED EndTime=%v", foundAuction, newEnd)
	}
}

func TestAuctionRepository_ListActiveEndingBefore(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestLegendaryItem(t, ctx, tx, "Auction Test Item 4")
	repo := NewAuctionRepository()

	now := time.Now().UTC()
	// Ends in the past relative to `cutoff` below: eligible for settlement.
	createdAuction, err := repo.Create(ctx, tx, Auction{
		ItemID: item.ID, OwnerGuildID: guild.ID, Status: AuctionActive,
		StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Minute), BasePrice: 9000,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Variable created for more readable code.
	cutoff := now
	list, err := repo.ListActiveEndingBefore(ctx, tx, cutoff)
	if err != nil {
		t.Fatalf("ListActiveEndingBefore() error = %v", err)
	}
	var found bool
	for _, a := range list {
		if a.ID == createdAuction.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListActiveEndingBefore(%v) did not include auction %d ending at %v", cutoff, createdAuction.ID, createdAuction.EndTime)
	}
}

func TestAuctionRepository_ListActive(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestLegendaryItem(t, ctx, tx, "Auction Test Item 5")
	repo := NewAuctionRepository()

	before, err := repo.ListActive(ctx, tx)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}

	start := time.Now().UTC()
	if _, err := repo.Create(ctx, tx, Auction{
		ItemID: item.ID, OwnerGuildID: guild.ID, Status: AuctionActive,
		StartTime: start, EndTime: start.Add(time.Hour), BasePrice: 9000,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	after, err := repo.ListActive(ctx, tx)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("ListActive() len = %d, want %d", len(after), len(before)+1)
	}
}
