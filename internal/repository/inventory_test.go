package repository

import (
	"context"
	"errors"
	"testing"
)

func createTestItem(t *testing.T, ctx context.Context, tx DBTX, name string, rarity ItemRarity, price int) (Item, Guild) {
	t.Helper()
	leader := createTestCharacter(t, ctx, tx, name+" Leader")
	guild, err := NewGuildRepository().Create(ctx, tx, Guild{
		Name:              name + " Guild",
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guild for %q: %v", name, err)
	}
	forger := createTestCharacter(t, ctx, tx, name+" Forger")
	item, err := NewItemRepository().Create(ctx, tx, Item{
		Name:              name,
		LandOfOrigin:      "Testland",
		Rarity:            rarity,
		ForgerCharacterID: forger.ID,
		Price:             price,
	})
	if err != nil {
		t.Fatalf("fixture: create item %q: %v", name, err)
	}
	return item, guild
}

func TestInventoryRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Inv Test Common", RarityCommon, 10)

	repo := NewInventoryRepository()
	createdInventory, err := repo.Create(ctx, tx, Inventory{
		GuildID:  guild.ID,
		ItemID:   item.ID,
		Quantity: 5,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdInventory.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}
	// item_rarity is trigger-populated; Create must not set it itself, but
	// the returned row should reflect the trigger's value.
	if createdInventory.ItemRarity != RarityCommon {
		t.Errorf("Create().ItemRarity = %q, want COMMON (trigger-populated)", createdInventory.ItemRarity)
	}

	fetchedInventory, err := repo.GetByGuildAndItem(ctx, tx, guild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if fetchedInventory.Quantity != 5 || fetchedInventory.ItemRarity != RarityCommon {
		t.Errorf("GetByGuildAndItem() = %+v, want Quantity=5 ItemRarity=COMMON", fetchedInventory)
	}
}

func TestInventoryRepository_GetByGuildAndItem_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewInventoryRepository()

	_, err := repo.GetByGuildAndItem(ctx, tx, 999999999, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByGuildAndItem() error = %v, want ErrNotFound", err)
	}
}

func TestInventoryRepository_GetForUpdate(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Inv Test ForUpdate", RarityCommon, 10)
	repo := NewInventoryRepository()

	if _, err := repo.Create(ctx, tx, Inventory{GuildID: guild.ID, ItemID: item.ID, Quantity: 3}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	fetchedInventory, err := repo.GetByGuildAndItemForUpdate(ctx, tx, guild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItemForUpdate() error = %v", err)
	}
	if fetchedInventory.Quantity != 3 {
		t.Errorf("GetByGuildAndItemForUpdate().Quantity = %d, want 3", fetchedInventory.Quantity)
	}
}

func TestInventoryRepository_UpdateQuantity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Inv Test Update", RarityCommon, 10)
	repo := NewInventoryRepository()

	if _, err := repo.Create(ctx, tx, Inventory{GuildID: guild.ID, ItemID: item.ID, Quantity: 8}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.UpdateQuantity(ctx, tx, guild.ID, item.ID, 2); err != nil {
		t.Fatalf("UpdateQuantity() error = %v", err)
	}

	fetchedInventory, err := repo.GetByGuildAndItem(ctx, tx, guild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if fetchedInventory.Quantity != 2 {
		t.Errorf("GetByGuildAndItem().Quantity = %d, want 2", fetchedInventory.Quantity)
	}
}

func TestInventoryRepository_Upsert(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guildA := createTestItem(t, ctx, tx, "Inv Test Upsert", RarityRare, 500)
	// A second guild for the "insert" half of upsert (transfer target).
	leaderB := createTestCharacter(t, ctx, tx, "Inv Upsert Leader B")
	guildB, err := NewGuildRepository().Create(ctx, tx, Guild{
		Name:              "Inv Upsert Guild B",
		LeaderCharacterID: leaderB.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guildB: %v", err)
	}

	repo := NewInventoryRepository()

	// First upsert call for (guildA, item) is an insert.
	inv, err := repo.Upsert(ctx, tx, guildA.ID, item.ID, 1)
	if err != nil {
		t.Fatalf("Upsert() insert error = %v", err)
	}
	if inv.Quantity != 1 {
		t.Errorf("Upsert() insert Quantity = %d, want 1", inv.Quantity)
	}

	// Second upsert call for the same (guildA, item) is an update.
	inv, err = repo.Upsert(ctx, tx, guildA.ID, item.ID, 9)
	if err != nil {
		t.Fatalf("Upsert() update error = %v", err)
	}
	if inv.Quantity != 9 {
		t.Errorf("Upsert() update Quantity = %d, want 9", inv.Quantity)
	}

	// Upsert for guildB is a fresh insert (different guild, same item).
	invB, err := repo.Upsert(ctx, tx, guildB.ID, item.ID, 1)
	if err != nil {
		t.Fatalf("Upsert() for guildB error = %v", err)
	}
	if invB.GuildID != guildB.ID || invB.Quantity != 1 {
		t.Errorf("Upsert() for guildB = %+v, want GuildID=%d Quantity=1", invB, guildB.ID)
	}
}

func TestInventoryRepository_Delete(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Inv Test Delete", RarityCommon, 10)
	repo := NewInventoryRepository()

	createdInventory, err := repo.Create(ctx, tx, Inventory{GuildID: guild.ID, ItemID: item.ID, Quantity: 1})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, tx, createdInventory.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repo.GetByGuildAndItem(ctx, tx, guild.ID, item.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByGuildAndItem() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestInventoryRepository_ListByGuildID(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item1, guild := createTestItem(t, ctx, tx, "Inv Test List1", RarityCommon, 10)
	forger2 := createTestCharacter(t, ctx, tx, "Inv Test List2 Forger")
	item2, err := NewItemRepository().Create(ctx, tx, Item{
		Name: "Inv Test List2", LandOfOrigin: "Testland", Rarity: RarityCommon,
		ForgerCharacterID: forger2.ID, Price: 20,
	})
	if err != nil {
		t.Fatalf("fixture: create item2: %v", err)
	}

	repo := NewInventoryRepository()
	if _, err := repo.Create(ctx, tx, Inventory{GuildID: guild.ID, ItemID: item1.ID, Quantity: 1}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := repo.Create(ctx, tx, Inventory{GuildID: guild.ID, ItemID: item2.ID, Quantity: 2}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := repo.ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByGuildID() len = %d, want 2", len(list))
	}
}
