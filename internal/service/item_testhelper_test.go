package service

import (
	"context"
	"testing"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

func newTestItemService(db TxPool) *ItemService {
	return NewItemService(
		db,
		repository.NewItemRepository(),
		repository.NewListingRepository(),
		repository.NewInventoryRepository(),
		repository.NewGuildRepository(),
		newTestService(),
		oracle.NewCache(),
	)
}

func createTestCharacter(t *testing.T, ctx context.Context, db repository.DBTX, name string) repository.Character {
	t.Helper()
	c, err := repository.NewCharacterRepository().Create(ctx, db, repository.Character{
		Name:         name,
		LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create character %q: %v", name, err)
	}
	return c
}

func createTestListedItem(t *testing.T, ctx context.Context, db repository.DBTX, name string, rarity repository.ItemRarity, price, quantity int, guild repository.Guild) (repository.Item, repository.Listing) {
	t.Helper()
	forger := createTestCharacter(t, ctx, db, name+" Forger")
	item, err := repository.NewItemRepository().Create(ctx, db, repository.Item{
		Name:              name,
		LandOfOrigin:      "Testland",
		Rarity:            rarity,
		ForgerCharacterID: forger.ID,
		Price:             price,
	})
	if err != nil {
		t.Fatalf("fixture: create item %q: %v", name, err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, db, repository.Inventory{
		GuildID: guild.ID, ItemID: item.ID, Quantity: quantity,
	}); err != nil {
		t.Fatalf("fixture: create inventory for item %q: %v", name, err)
	}
	listing, err := repository.NewListingRepository().Create(ctx, db, repository.Listing{
		ItemID: item.ID, GuildID: guild.ID, Quantity: quantity, BasePrice: price, Status: repository.ListingActive,
	})
	if err != nil {
		t.Fatalf("fixture: create listing for item %q: %v", name, err)
	}
	return item, listing
}
