package service

import (
	"context"
	"testing"
	"time"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

func newTestAuctionService(db TxPool) *AuctionService {
	return NewAuctionService(
		db,
		repository.NewAuctionRepository(),
		repository.NewBidRepository(),
		repository.NewItemRepository(),
		repository.NewInventoryRepository(),
		newTestService(),
		oracle.NewCache(),
	)
}

func createTestLegendaryItemOwnedBy(t *testing.T, ctx context.Context, db repository.DBTX, name string, price int, owner repository.Guild) repository.Item {
	t.Helper()
	forger := createTestCharacter(t, ctx, db, name+" Forger")
	item, err := repository.NewItemRepository().Create(ctx, db, repository.Item{
		Name: name, LandOfOrigin: "Testland", Rarity: repository.RarityLegendary,
		ForgerCharacterID: forger.ID, Price: price,
	})
	if err != nil {
		t.Fatalf("fixture: create legendary item %q: %v", name, err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, db, repository.Inventory{
		GuildID: owner.ID, ItemID: item.ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("fixture: create inventory for item %q: %v", name, err)
	}
	return item
}

func createTestAuctionFixture(t *testing.T, ctx context.Context, db repository.DBTX, item repository.Item, owner repository.Guild, basePrice int, endTime time.Time) repository.Auction {
	t.Helper()
	endTime = endTime.UTC().Truncate(time.Microsecond)
	start := endTime.Add(-time.Hour)
	if now := time.Now().UTC().Truncate(time.Microsecond); start.After(now) {
		start = now
	}
	a, err := repository.NewAuctionRepository().Create(ctx, db, repository.Auction{
		ItemID: item.ID, OwnerGuildID: owner.ID, StartTime: start, EndTime: endTime, BasePrice: basePrice,
	})
	if err != nil {
		t.Fatalf("fixture: create auction for item %d: %v", item.ID, err)
	}
	return a
}
