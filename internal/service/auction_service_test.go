package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"DragonMarket/internal/repository"
)

// --- CreateAuction ---

func TestAuctionService_CreateAuction_RejectsNonPositiveDuration(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction BadDuration Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CreateAuction BadDuration Item", 9000, owner)
	svc := newTestAuctionService(tx)

	_, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 0})
	if !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("CreateAuction() error = %v, want ErrInvalidDuration", err)
	}
}

func TestAuctionService_CreateAuction_ItemNotFound_ReturnsErrItemNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction NotFound Owner")
	svc := newTestAuctionService(tx)

	_, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: 999999999, OwnerGuildID: owner.ID, DurationSeconds: 3600})
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("CreateAuction() error = %v, want ErrItemNotFound", err)
	}
}

func TestAuctionService_CreateAuction_RejectsNonOwner(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction NonOwner Owner")
	stranger := createTestGuild(t, ctx, tx, "CreateAuction NonOwner Stranger")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CreateAuction NonOwner Item", 9000, owner)
	svc := newTestAuctionService(tx)

	_, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: stranger.ID, DurationSeconds: 3600})
	if !errors.Is(err, ErrNotItemOwner) {
		t.Fatalf("CreateAuction() error = %v, want ErrNotItemOwner", err)
	}
}

func TestAuctionService_CreateAuction_RejectsNonLegendary(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction NonLegendary Owner")
	item, _ := createTestListedItem(t, ctx, tx, "CreateAuction NonLegendary Item", repository.RarityRare, 100, 5, owner)
	svc := newTestAuctionService(tx)

	_, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 3600})
	if !errors.Is(err, ErrItemNotLegendary) {
		t.Fatalf("CreateAuction() error = %v, want ErrItemNotLegendary", err)
	}
}

func TestAuctionService_CreateAuction_RejectsDuplicateActiveAuction(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction Duplicate Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CreateAuction Duplicate Item", 9000, owner)
	svc := newTestAuctionService(tx)

	if _, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 3600}); err != nil {
		t.Fatalf("first CreateAuction() error = %v", err)
	}

	_, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 3600})
	if !errors.Is(err, ErrAuctionAlreadyExists) {
		t.Fatalf("second CreateAuction() error = %v, want ErrAuctionAlreadyExists", err)
	}
}

func TestAuctionService_CreateAuction_Success_UsesDBPriceWhenNoCacheEntry(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction Success Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CreateAuction Success Item", 8000, owner)
	svc := newTestAuctionService(tx)

	before := time.Now().UTC()
	result, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 3600})
	if err != nil {
		t.Fatalf("CreateAuction() error = %v", err)
	}
	if result.BasePrice != 8000 {
		t.Errorf("CreateAuction().BasePrice = %d, want 8000 (DB fallback, no cache entry)", result.BasePrice)
	}
	if result.Status != repository.AuctionActive {
		t.Errorf("CreateAuction().Status = %q, want ACTIVE", result.Status)
	}
	if result.OwnerGuildID != owner.ID || result.ItemID != item.ID {
		t.Errorf("CreateAuction() = %+v, want OwnerGuildID=%d ItemID=%d", result, owner.ID, item.ID)
	}
	wantEnd := before.Add(3600 * time.Second)
	if result.EndTime.Before(wantEnd.Add(-2*time.Second)) || result.EndTime.After(wantEnd.Add(2*time.Second)) {
		t.Errorf("CreateAuction().EndTime = %v, want ~%v", result.EndTime, wantEnd)
	}

	// Verify persisted, not just returned.
	stored, err := repository.NewAuctionRepository().GetByID(ctx, tx, result.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.BasePrice != 8000 || stored.Status != repository.AuctionActive {
		t.Errorf("stored auction = %+v, want BasePrice=8000 Status=ACTIVE", stored)
	}
}

func TestAuctionService_CreateAuction_Success_UsesCachedPriceWhenPresent(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CreateAuction Cached Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CreateAuction Cached Item", 8000, owner)
	svc := newTestAuctionService(tx)
	svc.priceCache.Set(item.ID, 8500)

	result, err := svc.CreateAuction(ctx, CreateAuctionInput{ItemID: item.ID, OwnerGuildID: owner.ID, DurationSeconds: 3600})
	if err != nil {
		t.Fatalf("CreateAuction() error = %v", err)
	}
	if result.BasePrice != 8500 {
		t.Errorf("CreateAuction().BasePrice = %d, want 8500 (cache entry present)", result.BasePrice)
	}
}

// --- GetAuction ---

func TestAuctionService_GetAuction_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	svc := newTestAuctionService(tx)

	_, err := svc.GetAuction(ctx, 999999999)
	if !errors.Is(err, ErrAuctionNotFound) {
		t.Fatalf("GetAuction() error = %v, want ErrAuctionNotFound", err)
	}
}

func TestAuctionService_GetAuction_Expired_ReturnsErrAuctionNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "GetAuction Expired Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "GetAuction Expired Item", 9000, owner)
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 9000, time.Now().UTC().Add(time.Hour))
	auction.Status = repository.AuctionExpired
	if err := repository.NewAuctionRepository().Update(ctx, tx, auction); err != nil {
		t.Fatalf("fixture: expire auction: %v", err)
	}
	svc := newTestAuctionService(tx)

	_, err := svc.GetAuction(ctx, auction.ID)
	if !errors.Is(err, ErrAuctionNotFound) {
		t.Fatalf("GetAuction() on EXPIRED auction error = %v, want ErrAuctionNotFound (must not return expired data as if live)", err)
	}
}

func TestAuctionService_GetAuction_Active_ReturnsView(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "GetAuction Active Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "GetAuction Active Item", 9000, owner)
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 9000, time.Now().UTC().Add(time.Hour))
	svc := newTestAuctionService(tx)

	view, err := svc.GetAuction(ctx, auction.ID)
	if err != nil {
		t.Fatalf("GetAuction() error = %v", err)
	}
	if view.ID != auction.ID || view.BasePrice != 9000 || view.Status != repository.AuctionActive {
		t.Errorf("GetAuction() = %+v, want ID=%d BasePrice=9000 Status=ACTIVE", view, auction.ID)
	}
}

// --- ListActiveAuctions ---

func TestAuctionService_ListActiveAuctions_ExcludesExpired_AppliesPagination(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "ListActive Owner")
	svc := newTestAuctionService(tx)

	var activeIDs []int64
	for i := 0; i < 3; i++ {
		item := createTestLegendaryItemOwnedBy(t, ctx, tx, fmt.Sprintf("ListActive Item %d", i), 1000, owner)
		a := createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
		activeIDs = append(activeIDs, a.ID)
	}
	expiredItem := createTestLegendaryItemOwnedBy(t, ctx, tx, "ListActive Expired Item", 1000, owner)
	expiredAuction := createTestAuctionFixture(t, ctx, tx, expiredItem, owner, 1000, time.Now().UTC().Add(time.Hour))
	expiredAuction.Status = repository.AuctionExpired
	if err := repository.NewAuctionRepository().Update(ctx, tx, expiredAuction); err != nil {
		t.Fatalf("fixture: expire auction: %v", err)
	}

	all, err := svc.ListActiveAuctions(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListActiveAuctions() error = %v", err)
	}
	for _, v := range all {
		if v.ID == expiredAuction.ID {
			t.Errorf("ListActiveAuctions() included EXPIRED auction %d", expiredAuction.ID)
		}
	}

	// Pagination: limit=1 offset=1 over just our 3 fixture auctions is hard
	// to assert in isolation (ListActive has no WHERE scoping to this test),
	// so instead assert the general pagination contract: limit caps the
	// count, and a large offset returns nothing.
	limited, err := svc.ListActiveAuctions(ctx, 1, 0)
	if err != nil {
		t.Fatalf("ListActiveAuctions(limit=1) error = %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("ListActiveAuctions(limit=1) len = %d, want 1", len(limited))
	}

	beyond, err := svc.ListActiveAuctions(ctx, 10, 999999)
	if err != nil {
		t.Fatalf("ListActiveAuctions(offset=999999) error = %v", err)
	}
	if len(beyond) != 0 {
		t.Errorf("ListActiveAuctions(offset=999999) len = %d, want 0", len(beyond))
	}
}

// --- PlaceBid ---

func TestAuctionService_PlaceBid_NoActiveAuction_ReturnsErrAuctionNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid NoAuction Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid NoAuction Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid NoAuction Item", 9000, owner)
	svc := newTestAuctionService(tx)

	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 9000})
	if !errors.Is(err, ErrAuctionNotFound) {
		t.Fatalf("PlaceBid() error = %v, want ErrAuctionNotFound", err)
	}
}

func TestAuctionService_PlaceBid_RejectsSelfBid(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid SelfBid Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid SelfBid Item", 9000, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 9000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: owner.ID, Amount: 9000})
	if !errors.Is(err, ErrSelfBidNotAllowed) {
		t.Fatalf("PlaceBid() error = %v, want ErrSelfBidNotAllowed", err)
	}
}

func TestAuctionService_PlaceBid_RejectsPastEndTime_EvenIfStillMarkedActive(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid PastEnd Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid PastEnd Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid PastEnd Item", 9000, owner)
	// end_time is in the past, but status is still ACTIVE (Task 10's sweep
	// hasn't run yet) -- PlaceBid must independently enforce the cutoff.
	createTestAuctionFixture(t, ctx, tx, item, owner, 9000, time.Now().UTC().Add(-time.Minute))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 9000})
	if !errors.Is(err, ErrAuctionExpired) {
		t.Fatalf("PlaceBid() on past-end_time (still ACTIVE) auction error = %v, want ErrAuctionExpired", err)
	}
}

func TestAuctionService_PlaceBid_FirstBid_RequiresAtLeastBasePrice(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid Floor Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid Floor Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid Floor Item", 1000, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 999})
	if !errors.Is(err, ErrBidTooLow) {
		t.Fatalf("PlaceBid(999 < base 1000) error = %v, want ErrBidTooLow", err)
	}

	bid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000})
	if err != nil {
		t.Fatalf("PlaceBid(1000 == base) error = %v", err)
	}
	if bid.Amount != 1000 || bid.Status != repository.BidActive {
		t.Errorf("PlaceBid() = %+v, want Amount=1000 Status=ACTIVE", bid)
	}
}

func TestAuctionService_PlaceBid_SubsequentBid_Requires105PercentOfHighest_CeilRounding(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid CeilFloor Owner")
	bidderA := createTestGuild(t, ctx, tx, "PlaceBid CeilFloor BidderA")
	bidderB := createTestGuild(t, ctx, tx, "PlaceBid CeilFloor BidderB")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid CeilFloor Item", 101, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 101, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderA.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderB.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderA.ID, Amount: 101}); err != nil {
		t.Fatalf("first PlaceBid() error = %v", err)
	}

	// highest=101; 101*1.05=106.05 -> ceiling means the true floor is 107,
	// not 106 (a naive truncating 105/100 multiply would wrongly accept 106).
	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderB.ID, Amount: 106})
	if !errors.Is(err, ErrBidTooLow) {
		t.Fatalf("PlaceBid(106) against highest=101 error = %v, want ErrBidTooLow (105%% of 101 ceils to 107)", err)
	}

	bid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderB.ID, Amount: 107})
	if err != nil {
		t.Fatalf("PlaceBid(107) against highest=101 error = %v", err)
	}
	if bid.Amount != 107 {
		t.Errorf("PlaceBid() = %+v, want Amount=107", bid)
	}
}

func TestAuctionService_PlaceBid_Success_ReservesFundsInSameTransaction(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid Reserve Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid Reserve Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid Reserve Item", 500, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 500, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 1000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	bid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 500})
	if err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, bidder.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.ReservedBalance != 500 || pouch.UsableBalance != 500 {
		t.Errorf("bidder pouch = %+v, want ReservedBalance=500 UsableBalance=500", pouch)
	}

	storedBid, err := repository.NewBidRepository().GetByID(ctx, tx, bid.ID)
	if err != nil {
		t.Fatalf("GetByID(bid) error = %v", err)
	}
	if storedBid.Status != repository.BidActive || storedBid.Amount != 500 {
		t.Errorf("stored bid = %+v, want Status=ACTIVE Amount=500", storedBid)
	}
}

func TestAuctionService_PlaceBid_InsufficientBalance_PropagatesErrorAndDoesNotCreateBid(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid PoorBidder Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid PoorBidder Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid PoorBidder Item", 500, owner)
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 500, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 10, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 500})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("PlaceBid() error = %v, want ErrInsufficientBalance", err)
	}

	bids, err := repository.NewBidRepository().ListByAuctionID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	if len(bids) != 0 {
		t.Errorf("ListByAuctionID() = %+v, want no bids created on a rejected PlaceBid", bids)
	}
}

func TestAuctionService_PlaceBid_WithinLast5Minutes_ExtendsEndTime_RepeatableOnEachQualifyingBid(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid Extend Owner")
	bidderA := createTestGuild(t, ctx, tx, "PlaceBid Extend BidderA")
	bidderB := createTestGuild(t, ctx, tx, "PlaceBid Extend BidderB")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid Extend Item", 1000, owner)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderA.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderB.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})

	originalEnd := time.Now().UTC().Add(3 * time.Minute) // within the 5-minute window
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 1000, originalEnd)
	svc := newTestAuctionService(tx)

	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderA.ID, Amount: 1000}); err != nil {
		t.Fatalf("first PlaceBid() error = %v", err)
	}

	afterFirst, err := repository.NewAuctionRepository().GetByID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	wantAfterFirst := originalEnd.Add(5 * time.Minute)
	if !afterFirst.EndTime.Equal(wantAfterFirst.Truncate(time.Microsecond)) {
		t.Fatalf("EndTime after first qualifying bid = %v, want %v (extended by 5m)", afterFirst.EndTime, wantAfterFirst)
	}

	// Simulate more time having passed by pulling end_time back down to
	// within the window again (directly via the repository, the same
	// technique auction_test.go's own tests use for GetByIDForUpdate+Update)
	// -- this avoids a real multi-minute sleep in the test while still
	// proving the extension re-triggers on a second qualifying bid rather
	// than firing only once.
	simulated := afterFirst
	simulated.EndTime = time.Now().UTC().Add(2 * time.Minute).Truncate(time.Microsecond)
	if err := repository.NewAuctionRepository().Update(ctx, tx, simulated); err != nil {
		t.Fatalf("fixture: simulate elapsed time: %v", err)
	}

	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderB.ID, Amount: 1050}); err != nil {
		t.Fatalf("second PlaceBid() error = %v", err)
	}

	afterSecond, err := repository.NewAuctionRepository().GetByID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	wantAfterSecond := simulated.EndTime.Add(5 * time.Minute)
	if !afterSecond.EndTime.Equal(wantAfterSecond) {
		t.Errorf("EndTime after second qualifying bid = %v, want %v (extension repeats, not one-shot)", afterSecond.EndTime, wantAfterSecond)
	}
	if afterSecond.EndTime.Equal(afterFirst.EndTime) {
		t.Errorf("EndTime did not change on the second qualifying bid -- extension only fired once")
	}
}

func TestAuctionService_PlaceBid_OutsideExtensionWindow_DoesNotExtendEndTime(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "PlaceBid NoExtend Owner")
	bidder := createTestGuild(t, ctx, tx, "PlaceBid NoExtend Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "PlaceBid NoExtend Item", 1000, owner)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})

	originalEnd := time.Now().UTC().Add(time.Hour) // well outside the 5-minute window
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 1000, originalEnd)
	svc := newTestAuctionService(tx)

	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000}); err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	after, err := repository.NewAuctionRepository().GetByID(ctx, tx, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !after.EndTime.Equal(originalEnd.Truncate(time.Microsecond)) {
		t.Errorf("EndTime = %v, want unchanged %v (bid arrived outside the 5-minute window)", after.EndTime, originalEnd)
	}
}

// --- CancelBid ---

func TestAuctionService_CancelBid_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CancelBid NotFound Owner")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CancelBid NotFound Item", 1000, owner)
	svc := newTestAuctionService(tx)

	err := svc.CancelBid(ctx, CancelBidInput{ItemID: item.ID, BidID: 999999999, GuildID: owner.ID})
	if !errors.Is(err, ErrBidNotFound) {
		t.Fatalf("CancelBid() error = %v, want ErrBidNotFound", err)
	}
}

func TestAuctionService_CancelBid_RejectsWrongGuild(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CancelBid WrongGuild Owner")
	bidder := createTestGuild(t, ctx, tx, "CancelBid WrongGuild Bidder")
	otherBidder := createTestGuild(t, ctx, tx, "CancelBid WrongGuild OtherBidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CancelBid WrongGuild Item", 1000, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: otherBidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	// bidder bids, then places a second (higher) bid so their first isn't
	// the highest, so we're testing the ownership check in isolation from
	// the "is highest" rejection.
	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000}); err != nil {
		t.Fatalf("first PlaceBid() error = %v", err)
	}
	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: otherBidder.ID, Amount: 1050}); err != nil {
		t.Fatalf("second PlaceBid() error = %v", err)
	}
	lowBid, err := repository.NewBidRepository().GetHighestActiveByAuctionID(ctx, tx, mustActiveAuctionID(t, ctx, tx, item.ID))
	_ = lowBid
	_ = err

	bids, err := repository.NewBidRepository().ListByAuctionID(ctx, tx, mustActiveAuctionID(t, ctx, tx, item.ID))
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	var nonHighestBidID int64
	for _, b := range bids {
		if b.GuildID == bidder.ID {
			nonHighestBidID = b.ID
		}
	}
	if nonHighestBidID == 0 {
		t.Fatalf("could not locate bidder's own (non-highest) bid among %+v", bids)
	}

	err = svc.CancelBid(ctx, CancelBidInput{ItemID: item.ID, BidID: nonHighestBidID, GuildID: otherBidder.ID})
	if !errors.Is(err, ErrNotBidOwner) {
		t.Fatalf("CancelBid() by non-owning guild error = %v, want ErrNotBidOwner", err)
	}
}

func TestAuctionService_CancelBid_RejectsIfAuctionNotActive(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CancelBid NotActive Owner")
	bidder := createTestGuild(t, ctx, tx, "CancelBid NotActive Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CancelBid NotActive Item", 1000, owner)
	auction := createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	bid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000})
	if err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	auction.Status = repository.AuctionExpired
	if err := repository.NewAuctionRepository().Update(ctx, tx, auction); err != nil {
		t.Fatalf("fixture: expire auction: %v", err)
	}

	err = svc.CancelBid(ctx, CancelBidInput{ItemID: item.ID, BidID: bid.ID, GuildID: bidder.ID})
	if !errors.Is(err, ErrAuctionNotActive) {
		t.Fatalf("CancelBid() on non-ACTIVE auction error = %v, want ErrAuctionNotActive", err)
	}
}

func TestAuctionService_CancelBid_RejectsIfHighestBid(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CancelBid Highest Owner")
	bidder := createTestGuild(t, ctx, tx, "CancelBid Highest Bidder")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CancelBid Highest Item", 1000, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	bid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000})
	if err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	err = svc.CancelBid(ctx, CancelBidInput{ItemID: item.ID, BidID: bid.ID, GuildID: bidder.ID})
	if !errors.Is(err, ErrBidIsHighest) {
		t.Fatalf("CancelBid() on the current highest bid error = %v, want ErrBidIsHighest", err)
	}
}

func TestAuctionService_CancelBid_Success_ReleasesFundsAndMarksCancelled(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	owner := createTestGuild(t, ctx, tx, "CancelBid Success Owner")
	bidderLow := createTestGuild(t, ctx, tx, "CancelBid Success BidderLow")
	bidderHigh := createTestGuild(t, ctx, tx, "CancelBid Success BidderHigh")
	item := createTestLegendaryItemOwnedBy(t, ctx, tx, "CancelBid Success Item", 1000, owner)
	createTestAuctionFixture(t, ctx, tx, item, owner, 1000, time.Now().UTC().Add(time.Hour))
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderLow.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: bidderHigh.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	svc := newTestAuctionService(tx)

	lowBid, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderLow.ID, Amount: 1000})
	if err != nil {
		t.Fatalf("low PlaceBid() error = %v", err)
	}
	if _, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidderHigh.ID, Amount: 1050}); err != nil {
		t.Fatalf("high PlaceBid() error = %v", err)
	}

	if err := svc.CancelBid(ctx, CancelBidInput{ItemID: item.ID, BidID: lowBid.ID, GuildID: bidderLow.ID}); err != nil {
		t.Fatalf("CancelBid() error = %v", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, bidderLow.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.ReservedBalance != 0 {
		t.Errorf("bidderLow.ReservedBalance = %d, want 0 (released)", pouch.ReservedBalance)
	}

	storedBid, err := repository.NewBidRepository().GetByID(ctx, tx, lowBid.ID)
	if err != nil {
		t.Fatalf("GetByID(bid) error = %v", err)
	}
	if storedBid.Status != repository.BidCancelled {
		t.Errorf("storedBid.Status = %q, want CANCELLED", storedBid.Status)
	}

	// Now the (formerly) low bid is gone, so the high bid is still highest
	// and the low bidder's guild ID must no longer show up as an ACTIVE bid.
	highest, err := repository.NewBidRepository().GetHighestActiveByAuctionID(ctx, tx, storedBid.AuctionID)
	if err != nil {
		t.Fatalf("GetHighestActiveByAuctionID() error = %v", err)
	}
	if highest.GuildID != bidderHigh.ID {
		t.Errorf("highest.GuildID = %d, want %d (bidderHigh)", highest.GuildID, bidderHigh.ID)
	}
}

func mustActiveAuctionID(t *testing.T, ctx context.Context, db repository.DBTX, itemID int64) int64 {
	t.Helper()
	a, err := repository.NewAuctionRepository().GetActiveByItemID(ctx, db, itemID)
	if err != nil {
		t.Fatalf("GetActiveByItemID() error = %v", err)
	}
	return a.ID
}

// --- Concurrency (mandated by the brief) ---

// TestAuctionService_PlaceBid_ConcurrentBidsAtSameFloor_OnlyOneAccepted proves
// the auction row lock actually serializes bid acceptance: N goroutines,
// each in its OWN transaction against the shared pool, race to place a bid
// for the exact SAME amount (the minimum acceptable against the auction's
// base_price). Since acceptance of the first raises the floor to 105% of
// that amount for everyone after it, at most one of these equal-amount bids
// can ever be valid -- if the row lock were not real (e.g. read-then-write
// without FOR UPDATE), multiple goroutines could all read the same stale
// "no bids yet" state concurrently and all get accepted, producing two
// ACTIVE bids of equal amount that are (trivially) not 5% apart from one
// another, which is exactly the invariant under test.
func TestAuctionService_PlaceBid_ConcurrentBidsAtSameFloor_OnlyOneAccepted(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("Concurrent Bid Owner %d", suffix))
	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("Concurrent Bid Item %d", suffix), 1000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(time.Hour))

	const n = 6
	const bidAmount = 1000 // == base_price: the floor for the first accepted bid
	bidders := make([]repository.Guild, n)
	for i := 0; i < n; i++ {
		bidders[i] = createTestGuild(t, ctx, testPool, fmt.Sprintf("Concurrent Bid Bidder %d %d", i, suffix))
		createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{
			GuildID: bidders[i].ID, TotalBalance: 100000, DailySpendingLimit: 1000000,
		})
	}

	svc := newTestAuctionService(testPool)
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.PlaceBid(ctx, PlaceBidInput{ItemID: item.ID, GuildID: bidders[i].ID, Amount: bidAmount})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrBidTooLow):
			failures++
		default:
			t.Fatalf("goroutine returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d, want 1 (equal-amount concurrent bids must serialize to exactly one winner)", successes)
	}
	if failures != n-1 {
		t.Errorf("failures = %d, want %d", failures, n-1)
	}

	bids, err := repository.NewBidRepository().ListByAuctionID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	activeCount := 0
	for _, b := range bids {
		if b.Status == repository.BidActive {
			activeCount++
			if b.Amount != bidAmount {
				t.Errorf("active bid amount = %d, want %d", b.Amount, bidAmount)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("ACTIVE bids for auction = %d, want 1", activeCount)
	}

	highest, err := repository.NewBidRepository().GetHighestActiveByAuctionID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("GetHighestActiveByAuctionID() error = %v", err)
	}
	if highest.Amount != bidAmount {
		t.Errorf("final highest bid = %d, want %d", highest.Amount, bidAmount)
	}

	winnerPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, highest.GuildID)
	if err != nil {
		t.Fatalf("GetByGuildID(winner) error = %v", err)
	}
	if winnerPouch.ReservedBalance != bidAmount {
		t.Errorf("winner ReservedBalance = %d, want %d", winnerPouch.ReservedBalance, bidAmount)
	}

	for _, b := range bidders {
		if b.ID == highest.GuildID {
			continue
		}
		pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, b.ID)
		if err != nil {
			t.Fatalf("GetByGuildID(loser) error = %v", err)
		}
		if pouch.ReservedBalance != 0 {
			t.Errorf("loser guild %d ReservedBalance = %d, want 0 (rejected bid must not reserve funds)", b.ID, pouch.ReservedBalance)
		}
	}
}
