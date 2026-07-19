package repository

import (
	"context"
	"errors"
	"testing"
)

func TestListingRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Listing Test Item 1", RarityCommon, 30)

	repo := NewListingRepository()
	createdListing, err := repo.Create(ctx, tx, Listing{
		ItemID:    item.ID,
		GuildID:   guild.ID,
		Quantity:  10,
		BasePrice: 30,
		Status:    ListingActive,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdListing.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}

	fetchedListing, err := repo.GetByID(ctx, tx, createdListing.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedListing.Quantity != 10 || fetchedListing.Status != ListingActive {
		t.Errorf("GetByID() = %+v, want Quantity=10 Status=ACTIVE", fetchedListing)
	}
}

func TestListingRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewListingRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestListingRepository_GetActiveByItemID(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Listing Test Item 2", RarityCommon, 40)
	repo := NewListingRepository()

	createdListing, err := repo.Create(ctx, tx, Listing{
		ItemID: item.ID, GuildID: guild.ID, Quantity: 4, BasePrice: 40, Status: ListingActive,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	fetchedListing, err := repo.GetActiveByItemID(ctx, tx, item.ID)
	if err != nil {
		t.Fatalf("GetActiveByItemID() error = %v", err)
	}
	if fetchedListing.ID != createdListing.ID {
		t.Errorf("GetActiveByItemID().ID = %d, want %d", fetchedListing.ID, createdListing.ID)
	}
}

func TestListingRepository_GetByIDForUpdate_AndUpdate(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Listing Test Item 3", RarityCommon, 15)
	repo := NewListingRepository()

	createdListing, err := repo.Create(ctx, tx, Listing{
		ItemID: item.ID, GuildID: guild.ID, Quantity: 1, BasePrice: 15, Status: ListingActive,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	locked, err := repo.GetByIDForUpdate(ctx, tx, createdListing.ID)
	if err != nil {
		t.Fatalf("GetByIDForUpdate() error = %v", err)
	}
	locked.Quantity = 0
	locked.Status = ListingExpired

	if err := repo.Update(ctx, tx, locked); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	fetchedListing, err := repo.GetByID(ctx, tx, createdListing.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedListing.Quantity != 0 || fetchedListing.Status != ListingExpired {
		t.Errorf("GetByID() after Update() = %+v, want Quantity=0 Status=EXPIRED", fetchedListing)
	}
}

func TestListingRepository_ListByStatus(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	item, guild := createTestItem(t, ctx, tx, "Listing Test Item 4", RarityCommon, 22)
	repo := NewListingRepository()

	before, err := repo.ListByStatus(ctx, tx, ListingActive)
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}

	if _, err := repo.Create(ctx, tx, Listing{
		ItemID: item.ID, GuildID: guild.ID, Quantity: 3, BasePrice: 22, Status: ListingActive,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	after, err := repo.ListByStatus(ctx, tx, ListingActive)
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("ListByStatus(ACTIVE) len = %d, want %d", len(after), len(before)+1)
	}
}
