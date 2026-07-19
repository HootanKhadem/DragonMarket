package repository

import (
	"context"
	"errors"
	"testing"
)

func TestItemRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "Test Forger One")

	repo := NewItemRepository()
	createdItem, err := repo.Create(ctx, tx, Item{
		Name:              "Test Sword",
		LandOfOrigin:      "Testland",
		Rarity:            RarityRare,
		ForgerCharacterID: forger.ID,
		Price:             123,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdItem.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}

	fetchedItem, err := repo.GetByID(ctx, tx, createdItem.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	// This is the enum round-trip check: RarityRare must come back exactly
	// as RarityRare through the native item_rarity Postgres enum column,
	// with no cast/workaround needed at the call site.
	if fetchedItem.Rarity != RarityRare {
		t.Errorf("GetByID().Rarity = %q, want %q", fetchedItem.Rarity, RarityRare)
	}
	if fetchedItem.Name != "Test Sword" || fetchedItem.Price != 123 || fetchedItem.ForgerCharacterID != forger.ID {
		t.Errorf("GetByID() = %+v, want matching Name/Price/ForgerCharacterID", fetchedItem)
	}
}

func TestItemRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewItemRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestItemRepository_ListByRarity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "Test Forger Two")
	repo := NewItemRepository()

	beforeLegendary, err := repo.ListByRarity(ctx, tx, RarityLegendary)
	if err != nil {
		t.Fatalf("ListByRarity() error = %v", err)
	}

	createdItem, err := repo.Create(ctx, tx, Item{
		Name:              "Test Legendary Blade",
		LandOfOrigin:      "Testland",
		Rarity:            RarityLegendary,
		ForgerCharacterID: forger.ID,
		Price:             9999,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	afterLegendary, err := repo.ListByRarity(ctx, tx, RarityLegendary)
	if err != nil {
		t.Fatalf("ListByRarity() error = %v", err)
	}
	if len(afterLegendary) != len(beforeLegendary)+1 {
		t.Fatalf("ListByRarity(LEGENDARY) len = %d, want %d", len(afterLegendary), len(beforeLegendary)+1)
	}

	var found bool
	for _, it := range afterLegendary {
		if it.ID == createdItem.ID {
			found = true
			if it.Rarity != RarityLegendary {
				t.Errorf("item %d Rarity = %q, want LEGENDARY", it.ID, it.Rarity)
			}
		}
	}
	if !found {
		t.Errorf("ListByRarity(LEGENDARY) did not include created item %d", createdItem.ID)
	}
}

func TestItemRepository_UpdatePrice(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "Test Forger Three")
	repo := NewItemRepository()

	createdItem, err := repo.Create(ctx, tx, Item{
		Name:              "Test Price Item",
		LandOfOrigin:      "Testland",
		Rarity:            RarityCommon,
		ForgerCharacterID: forger.ID,
		Price:             50,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.UpdatePrice(ctx, tx, createdItem.ID, 75); err != nil {
		t.Fatalf("UpdatePrice() error = %v", err)
	}

	fetchedItem, err := repo.GetByID(ctx, tx, createdItem.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedItem.Price != 75 {
		t.Errorf("GetByID().Price = %d, want 75", fetchedItem.Price)
	}
}
