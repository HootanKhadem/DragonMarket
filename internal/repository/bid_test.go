package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createTestAuction(t *testing.T, ctx context.Context, tx DBTX, name string) (Auction, Item, Guild) {
	t.Helper()
	item, guild := createTestLegendaryItem(t, ctx, tx, name)
	start := time.Now().UTC()
	auction, err := NewAuctionRepository().Create(ctx, tx, Auction{
		ItemID: item.ID, OwnerGuildID: guild.ID, Status: AuctionActive,
		StartTime: start, EndTime: start.Add(time.Hour), BasePrice: item.Price,
	})
	if err != nil {
		t.Fatalf("fixture: create auction for %q: %v", name, err)
	}
	return auction, item, guild
}

func TestBidRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	auction, _, _ := createTestAuction(t, ctx, tx, "Bid Test Item 1")
	bidder := createTestGuild(t, ctx, tx, "Bid Test Bidder 1")

	repo := NewBidRepository()
	createdBid, err := repo.Create(ctx, tx, Bid{
		AuctionID: auction.ID,
		GuildID:   bidder.ID,
		Amount:    9500,
		Status:    BidActive,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdBid.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}
	if createdBid.CreatedAt.IsZero() {
		t.Errorf("Create().CreatedAt is zero, want default now()")
	}

	fetchedBid, err := repo.GetByID(ctx, tx, createdBid.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedBid.Amount != 9500 || fetchedBid.Status != BidActive || fetchedBid.AuctionID != auction.ID {
		t.Errorf("GetByID() = %+v, want Amount=9500 Status=ACTIVE AuctionID=%d", fetchedBid, auction.ID)
	}
}

func TestBidRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewBidRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestBidRepository_GetHighestActiveByAuctionID(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	auction, _, _ := createTestAuction(t, ctx, tx, "Bid Test Item 2")
	bidderLow := createTestGuild(t, ctx, tx, "Bid Test Bidder Low")
	bidderHigh := createTestGuild(t, ctx, tx, "Bid Test Bidder High")
	repo := NewBidRepository()

	if _, err := repo.Create(ctx, tx, Bid{AuctionID: auction.ID, GuildID: bidderLow.ID, Amount: 9200, Status: BidActive}); err != nil {
		t.Fatalf("Create() low bid error = %v", err)
	}
	highest, err := repo.Create(ctx, tx, Bid{AuctionID: auction.ID, GuildID: bidderHigh.ID, Amount: 9800, Status: BidActive})
	if err != nil {
		t.Fatalf("Create() high bid error = %v", err)
	}

	fetchedBid, err := repo.GetHighestActiveByAuctionID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("GetHighestActiveByAuctionID() error = %v", err)
	}
	if fetchedBid.ID != highest.ID || fetchedBid.Amount != 9800 {
		t.Errorf("GetHighestActiveByAuctionID() = %+v, want ID=%d Amount=9800", fetchedBid, highest.ID)
	}
}

func TestBidRepository_GetHighestActiveByAuctionID_NoBids(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	auction, _, _ := createTestAuction(t, ctx, tx, "Bid Test Item 3")
	repo := NewBidRepository()

	_, err := repo.GetHighestActiveByAuctionID(ctx, tx, auction.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetHighestActiveByAuctionID() error = %v, want ErrNotFound", err)
	}
}

func TestBidRepository_ListByAuctionID_AndUpdate(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	auction, _, _ := createTestAuction(t, ctx, tx, "Bid Test Item 4")
	bidder := createTestGuild(t, ctx, tx, "Bid Test Bidder 4")
	repo := NewBidRepository()

	createdBid, err := repo.Create(ctx, tx, Bid{AuctionID: auction.ID, GuildID: bidder.ID, Amount: 9300, Status: BidActive})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	createdBid.Status = BidWon
	if err := repo.Update(ctx, tx, createdBid); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	list, err := repo.ListByAuctionID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	if len(list) != 1 || list[0].Status != BidWon {
		t.Errorf("ListByAuctionID() = %+v, want one bid with Status=WON", list)
	}
}
