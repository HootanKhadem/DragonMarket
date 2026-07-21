package settlement

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

// --- SettleAuction: no bids ---

func TestSettler_SettleAuction_NoBids_ExpiresAuctionWithoutMutatingFundsOrInventory(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("NoBids Owner %d", suffix))
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 5000, DailySpendingLimit: 1000000})
	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("NoBids Item %d", suffix), 9000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 9000, time.Now().UTC().Add(-time.Minute))

	settler := newTestSettler()
	if err := settler.SettleAuction(ctx, auction.ID); err != nil {
		t.Fatalf("SettleAuction() error = %v", err)
	}

	stored, err := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.Status != repository.AuctionExpired {
		t.Errorf("auction.Status = %q, want EXPIRED", stored.Status)
	}

	inv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, testPool, owner.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v (item must remain with owner)", err)
	}
	if inv.Quantity != 1 {
		t.Errorf("owner inventory quantity = %d, want 1 (unchanged)", inv.Quantity)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, owner.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.TotalBalance != 5000 {
		t.Errorf("owner.TotalBalance = %d, want unchanged 5000 (no bids means no funds movement)", pouch.TotalBalance)
	}
}

// --- SettleAuction: with a winner ---

func TestSettler_SettleAuction_WithWinner_TransfersFundsAndOwnership_ReleasesLosers(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("Winner Owner %d", suffix))
	winnerGuild := createTestGuild(t, ctx, testPool, fmt.Sprintf("Winner Bidder %d", suffix))
	loserA := createTestGuild(t, ctx, testPool, fmt.Sprintf("Winner LoserA %d", suffix))
	loserB := createTestGuild(t, ctx, testPool, fmt.Sprintf("Winner LoserB %d", suffix))

	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 0, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: winnerGuild.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: loserA.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: loserB.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})

	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("Winner Item %d", suffix), 1000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(time.Hour))

	svc := newTestAuctionService()
	if _, err := svc.PlaceBid(ctx, service.PlaceBidInput{ItemID: item.ID, GuildID: loserA.ID, Amount: 1000}); err != nil {
		t.Fatalf("PlaceBid(loserA) error = %v", err)
	}
	if _, err := svc.PlaceBid(ctx, service.PlaceBidInput{ItemID: item.ID, GuildID: loserB.ID, Amount: 1100}); err != nil {
		t.Fatalf("PlaceBid(loserB) error = %v", err)
	}
	winningBid, err := svc.PlaceBid(ctx, service.PlaceBidInput{ItemID: item.ID, GuildID: winnerGuild.ID, Amount: 1500})
	if err != nil {
		t.Fatalf("PlaceBid(winner) error = %v", err)
	}

	expiredRow, err := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	expiredRow.EndTime = expiredRow.StartTime.Add(time.Millisecond)
	if err := repository.NewAuctionRepository().Update(ctx, testPool, expiredRow); err != nil {
		t.Fatalf("fixture: push end_time into the past: %v", err)
	}

	settler := newTestSettler()
	if err := settler.SettleAuction(ctx, auction.ID); err != nil {
		t.Fatalf("SettleAuction() error = %v", err)
	}

	// Auction EXPIRED.
	stored, err := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.Status != repository.AuctionExpired {
		t.Errorf("auction.Status = %q, want EXPIRED", stored.Status)
	}

	// Winner's pouch: reserved funds moved to spent, total decremented.
	winnerPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, winnerGuild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID(winner) error = %v", err)
	}
	if winnerPouch.ReservedBalance != 0 {
		t.Errorf("winner.ReservedBalance = %d, want 0", winnerPouch.ReservedBalance)
	}
	if winnerPouch.TotalBalance != 100000-1500 {
		t.Errorf("winner.TotalBalance = %d, want %d", winnerPouch.TotalBalance, 100000-1500)
	}
	if winnerPouch.SpentToday != 1500 {
		t.Errorf("winner.SpentToday = %d, want 1500", winnerPouch.SpentToday)
	}

	// Seller credited with the winning amount.
	sellerPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, owner.ID)
	if err != nil {
		t.Fatalf("GetByGuildID(owner) error = %v", err)
	}
	if sellerPouch.TotalBalance != 1500 {
		t.Errorf("seller.TotalBalance = %d, want 1500", sellerPouch.TotalBalance)
	}

	// Ownership transferred: seller's inventory row gone, winner's present at qty 1.
	if _, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, testPool, owner.ID, item.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("seller inventory GetByGuildAndItem() error = %v, want ErrNotFound (row must be deleted, not zeroed)", err)
	}
	winnerInv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, testPool, winnerGuild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem(winner) error = %v", err)
	}
	if winnerInv.Quantity != 1 {
		t.Errorf("winner inventory quantity = %d, want 1", winnerInv.Quantity)
	}

	// Bid statuses: winner WON, both losers LOST with reservations released.
	bids, err := repository.NewBidRepository().ListByAuctionID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	if len(bids) != 3 {
		t.Fatalf("len(bids) = %d, want 3", len(bids))
	}
	for _, b := range bids {
		switch b.ID {
		case winningBid.ID:
			if b.Status != repository.BidWon {
				t.Errorf("winning bid status = %q, want WON", b.Status)
			}
		default:
			if b.Status != repository.BidLost {
				t.Errorf("bid %d (guild %d) status = %q, want LOST", b.ID, b.GuildID, b.Status)
			}
		}
	}

	for _, loser := range []repository.Guild{loserA, loserB} {
		pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, loser.ID)
		if err != nil {
			t.Fatalf("GetByGuildID(loser %d) error = %v", loser.ID, err)
		}
		if pouch.ReservedBalance != 0 {
			t.Errorf("loser %d ReservedBalance = %d, want 0 (released)", loser.ID, pouch.ReservedBalance)
		}
		if pouch.TotalBalance != 100000 {
			t.Errorf("loser %d TotalBalance = %d, want unchanged 100000", loser.ID, pouch.TotalBalance)
		}
	}
}

// --- SettleAuction: re-check under the lock ---

func TestSettler_SettleAuction_AlreadyExpired_ReturnsErrAuctionNotEligible(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("AlreadyExpired Owner %d", suffix))
	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("AlreadyExpired Item %d", suffix), 1000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(-time.Minute))
	auction.Status = repository.AuctionExpired
	if err := repository.NewAuctionRepository().Update(ctx, testPool, auction); err != nil {
		t.Fatalf("fixture: pre-expire auction: %v", err)
	}

	settler := newTestSettler()
	err := settler.SettleAuction(ctx, auction.ID)
	if !errors.Is(err, ErrAuctionNotEligible) {
		t.Fatalf("SettleAuction() on already-EXPIRED auction error = %v, want ErrAuctionNotEligible", err)
	}
}

func TestSettler_SettleAuction_EndTimeInFuture_ReturnsErrAuctionNotEligible(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("NotYetEnded Owner %d", suffix))
	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("NotYetEnded Item %d", suffix), 1000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(time.Hour))

	settler := newTestSettler()
	err := settler.SettleAuction(ctx, auction.ID)
	if !errors.Is(err, ErrAuctionNotEligible) {
		t.Fatalf("SettleAuction() on not-yet-ended auction error = %v, want ErrAuctionNotEligible", err)
	}

	stored, getErr := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
	if getErr != nil {
		t.Fatalf("GetByID() error = %v", getErr)
	}
	if stored.Status != repository.AuctionActive {
		t.Errorf("auction.Status = %q, want unchanged ACTIVE (must not force-settle)", stored.Status)
	}
}

// --- Tick ---

func TestSettler_Tick_SettlesEligibleAuctions_SkipsNotYetEnded(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("Tick Owner %d", suffix))
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 0, DailySpendingLimit: 1000000})

	eligibleItem := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("Tick Eligible Item %d", suffix), 1000, owner)
	eligibleAuction := createTestAuctionFixture(t, ctx, testPool, eligibleItem, owner, 1000, time.Now().UTC().Add(-time.Minute))

	notYetEndedItem := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("Tick NotYetEnded Item %d", suffix), 1000, owner)
	notYetEndedAuction := createTestAuctionFixture(t, ctx, testPool, notYetEndedItem, owner, 1000, time.Now().UTC().Add(time.Hour))

	settler := newTestSettler()
	if err := settler.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	settled, err := repository.NewAuctionRepository().GetByID(ctx, testPool, eligibleAuction.ID)
	if err != nil {
		t.Fatalf("GetByID(eligible) error = %v", err)
	}
	if settled.Status != repository.AuctionExpired {
		t.Errorf("eligible auction.Status = %q, want EXPIRED", settled.Status)
	}

	untouched, err := repository.NewAuctionRepository().GetByID(ctx, testPool, notYetEndedAuction.ID)
	if err != nil {
		t.Fatalf("GetByID(not yet ended) error = %v", err)
	}
	if untouched.Status != repository.AuctionActive {
		t.Errorf("not-yet-ended auction.Status = %q, want still ACTIVE", untouched.Status)
	}
}

func TestSettler_ConcurrentLateBidVsSweep_BidRejectedSettlementWins(t *testing.T) {
	ctx := context.Background()
	svc := newTestAuctionService()
	settler := newTestSettler()

	for i := range 5 {
		suffix := fmt.Sprintf("%d-%d", time.Now().UnixNano(), i)
		owner := createTestGuild(t, ctx, testPool, "LateBid Owner "+suffix)
		bidder := createTestGuild(t, ctx, testPool, "LateBid Bidder "+suffix)
		createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 0, DailySpendingLimit: 1000000})
		createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
		item := createTestLegendaryItemOwnedBy(t, ctx, testPool, "LateBid Item "+suffix, 1000, owner)
		auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(-2*time.Second))

		var wg sync.WaitGroup
		var bidErr, settleErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, bidErr = svc.PlaceBid(ctx, service.PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000})
		}()
		go func() {
			defer wg.Done()
			settleErr = settler.SettleAuction(ctx, auction.ID)
		}()
		wg.Wait()

		if bidErr == nil {
			t.Fatalf("trial %d: PlaceBid() on an auction 2s past end_time unexpectedly succeeded", i)
		}
		if !errors.Is(bidErr, service.ErrAuctionExpired) && !errors.Is(bidErr, service.ErrAuctionNotActive) && !errors.Is(bidErr, service.ErrAuctionNotFound) {
			t.Fatalf("trial %d: PlaceBid() error = %v, want ErrAuctionExpired, ErrAuctionNotActive, or ErrAuctionNotFound", i, bidErr)
		}
		if settleErr != nil {
			t.Fatalf("trial %d: SettleAuction() error = %v", i, settleErr)
		}

		stored, err := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
		if err != nil {
			t.Fatalf("trial %d: GetByID() error = %v", i, err)
		}
		if stored.Status != repository.AuctionExpired {
			t.Errorf("trial %d: auction.Status = %q, want EXPIRED", i, stored.Status)
		}

		bids, err := repository.NewBidRepository().ListByAuctionID(ctx, testPool, auction.ID)
		if err != nil {
			t.Fatalf("trial %d: ListByAuctionID() error = %v", i, err)
		}
		if len(bids) != 0 {
			t.Errorf("trial %d: bids = %+v, want none (the rejected bid must never have been persisted)", i, bids)
		}

		bidderPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, bidder.ID)
		if err != nil {
			t.Fatalf("trial %d: GetByGuildID(bidder) error = %v", i, err)
		}
		if bidderPouch.ReservedBalance != 0 {
			t.Errorf("trial %d: bidder.ReservedBalance = %d, want 0 (no orphaned reservation)", i, bidderPouch.ReservedBalance)
		}

		// No winner: item stays with the seller, seller pouch untouched.
		if _, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, testPool, owner.ID, item.ID); err != nil {
			t.Errorf("trial %d: seller inventory GetByGuildAndItem() error = %v, want item to remain with seller", i, err)
		}
	}
}

func TestSettler_ConcurrentLastSecondBidVsSweep_BidExtendsSettlementSkips(t *testing.T) {
	ctx := context.Background()
	svc := newTestAuctionService()
	settler := newTestSettler()

	suffix := time.Now().UnixNano()
	owner := createTestGuild(t, ctx, testPool, fmt.Sprintf("LastSecond Owner %d", suffix))
	bidder := createTestGuild(t, ctx, testPool, fmt.Sprintf("LastSecond Bidder %d", suffix))
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: owner.ID, TotalBalance: 0, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: bidder.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	item := createTestLegendaryItemOwnedBy(t, ctx, testPool, fmt.Sprintf("LastSecond Item %d", suffix), 1000, owner)
	auction := createTestAuctionFixture(t, ctx, testPool, item, owner, 1000, time.Now().UTC().Add(2*time.Second))

	var wg sync.WaitGroup
	var bidErr, settleErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, bidErr = svc.PlaceBid(ctx, service.PlaceBidInput{ItemID: item.ID, GuildID: bidder.ID, Amount: 1000})
	}()
	go func() {
		defer wg.Done()
		settleErr = settler.SettleAuction(ctx, auction.ID)
	}()
	wg.Wait()

	if bidErr != nil {
		t.Fatalf("PlaceBid() error = %v, want success (bid lands before end_time)", bidErr)
	}
	if !errors.Is(settleErr, ErrAuctionNotEligible) {
		t.Fatalf("SettleAuction() error = %v, want ErrAuctionNotEligible (end_time was pushed out from under it)", settleErr)
	}

	stored, err := repository.NewAuctionRepository().GetByID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.Status != repository.AuctionActive {
		t.Errorf("auction.Status = %q, want still ACTIVE (sweep must not force-settle a just-extended auction)", stored.Status)
	}
	if !stored.EndTime.After(time.Now().UTC()) {
		t.Errorf("auction.EndTime = %v, want extended into the future", stored.EndTime)
	}

	bids, err := repository.NewBidRepository().ListByAuctionID(ctx, testPool, auction.ID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	if len(bids) != 1 || bids[0].Status != repository.BidActive {
		t.Fatalf("bids = %+v, want exactly one ACTIVE bid", bids)
	}

	bidderPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, bidder.ID)
	if err != nil {
		t.Fatalf("GetByGuildID(bidder) error = %v", err)
	}
	if bidderPouch.ReservedBalance != 1000 {
		t.Errorf("bidder.ReservedBalance = %d, want 1000 (bid accepted, not settled/released)", bidderPouch.ReservedBalance)
	}
}

// --- Run (ticker loop) ---

func TestSettler_Run_TicksRepeatedlyUntilContextCanceled(t *testing.T) {
	settler := newTestSettler()
	settler.interval = 5 * time.Millisecond

	var tickCount int32
	settler.OnTick = func(error) { atomic.AddInt32(&tickCount, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		settler.Run(ctx)
		close(done)
	}()

	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	if atomic.LoadInt32(&tickCount) < 2 {
		t.Errorf("tickCount = %d, want at least 2 ticks in 60ms at a 5ms interval", tickCount)
	}
}
