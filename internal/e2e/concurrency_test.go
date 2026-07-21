package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/settlement"
)

func TestConcurrency_PurchaseRace_NeverOversells(t *testing.T) {
	ctx := context.Background()
	qty := 3
	created := createItemHTTP(t, ctx, "RARE", 700, &qty, nil)
	itemID := created.Item.ID

	buyers := make([]int64, 4)
	for i := range buyers {
		buyers[i] = createGuildWithPouch(t, ctx, fmt.Sprintf("PurchaseRace Buyer %d", i), 100000, 1000000)
	} // 4 concurrent buyers racing 3 units.

	var wg sync.WaitGroup
	statuses := make([]int, len(buyers))
	bodies := make([][]byte, len(buyers))
	errs := make([]error, len(buyers))
	for i, guildID := range buyers {
		wg.Add(1)
		go func(i int, guildID int64) {
			defer wg.Done()
			statuses[i], bodies[i], errs[i] = doJSONRaw(http.MethodPost,
				fmt.Sprintf("/items/%d/purchase", itemID), &guildID, map[string]any{"quantity": 1})
		}(i, guildID)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	successes, failures := 0, 0
	for i, status := range statuses {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			failures++

			switch code := errorCode(t, bodies[i]); code {
			case "INSUFFICIENT_QUANTITY", "LISTING_NOT_ACTIVE":
			default:
				t.Errorf("goroutine %d: error code = %q, want INSUFFICIENT_QUANTITY or LISTING_NOT_ACTIVE", i, code)
			}
		default:
			t.Fatalf("goroutine %d: unexpected status %d; body = %s", i, status, bodies[i])
		}
	}
	if successes != 3 {
		t.Errorf("successes = %d, want 3 (never oversell a qty-3 listing)", successes)
	}
	if failures != len(buyers)-3 {
		t.Errorf("failures = %d, want %d", failures, len(buyers)-3)
	}

	// The listing must now read exactly sold-out: quantity 0, EXPIRED.
	var quantity int
	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT quantity, status FROM listings WHERE item_id = $1`, itemID,
	).Scan(&quantity, &status); err != nil {
		t.Fatalf("query final listing state: %v", err)
	}
	if quantity != 0 {
		t.Errorf("final listing quantity = %d, want 0", quantity)
	}
	if status != "EXPIRED" {
		t.Errorf("final listing status = %q, want EXPIRED", status)
	}

	// A further purchase attempt must now be rejected as sold out.
	extraBuyer := buyers[0]
	status2, raw := doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/purchase", itemID), &extraBuyer,
		map[string]any{"quantity": 1})
	if status2 != http.StatusConflict {
		t.Errorf("post-race purchase: status = %d, want 409; body = %s", status2, raw)
	}
	if code := errorCode(t, raw); code != "LISTING_NOT_ACTIVE" {
		t.Errorf("post-race purchase: error code = %q, want LISTING_NOT_ACTIVE", code)
	}
}

func TestConcurrency_BidRace_AtSameFloor_OnlyOneAccepted(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "BidRace Owner", 0, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 1000, nil, &owner)

	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 3600,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	auction := decodeInto[auctionDTO](t, raw)

	const n = 6
	const bidAmount = 1000 // == base_price: the floor for the FIRST accepted bid
	bidders := make([]int64, n)
	for i := range n {
		bidders[i] = createGuildWithPouch(t, ctx, fmt.Sprintf("BidRace Bidder %d", i), 100000, 1000000)
	}

	var wg sync.WaitGroup
	statuses := make([]int, n)
	bodies := make([][]byte, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], bodies[i], errs[i] = doJSONRaw(http.MethodPost,
				fmt.Sprintf("/items/%d/bid", item.Item.ID), &bidders[i], map[string]any{"amount": bidAmount})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	successes, failures := 0, 0
	for i, status := range statuses {
		switch status {
		case http.StatusCreated:
			successes++
		case http.StatusConflict:
			failures++
			if code := errorCode(t, bodies[i]); code != "BID_TOO_LOW" {
				t.Errorf("goroutine %d: error code = %q, want BID_TOO_LOW", i, code)
			}
		default:
			t.Fatalf("goroutine %d: unexpected status %d; body = %s", i, status, bodies[i])
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d, want 1 (equal-amount concurrent bids must serialize to exactly one winner)", successes)
	}
	if failures != n-1 {
		t.Errorf("failures = %d, want %d", failures, n-1)
	}

	bids, err := bidRepo.ListByAuctionID(ctx, testPool, auction.ID)
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
}

func TestConcurrency_LateBidVsSweep_BidRejectedSettlementWins(t *testing.T) {
	ctx := context.Background()

	for trial := range 5 {
		owner := createGuildWithPouch(t, ctx, fmt.Sprintf("LateBid Owner %d", trial), 0, 1000000)
		bidder := createGuildWithPouch(t, ctx, fmt.Sprintf("LateBid Bidder %d", trial), 100000, 1000000)
		item := createItemHTTP(t, ctx, "LEGENDARY", 1000, nil, &owner)
		auctionID := createAuctionFixture(t, ctx, item.Item.ID, owner, 1000, time.Now().UTC().Add(-2*time.Second))

		var wg sync.WaitGroup
		var bidStatus int
		var bidBody []byte
		var bidErr, settleErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			bidStatus, bidBody, bidErr = doJSONRaw(http.MethodPost,
				fmt.Sprintf("/items/%d/bid", item.Item.ID), &bidder, map[string]any{"amount": 1000})
		}()
		go func() {
			defer wg.Done()
			settleErr = settler.SettleAuction(ctx, auctionID)
		}()
		wg.Wait()
		if bidErr != nil {
			t.Fatalf("trial %d: bid request: %v", trial, bidErr)
		}

		if bidStatus != http.StatusConflict {
			t.Fatalf("trial %d: bid on a 2s-expired auction: status = %d, want 409; body = %s", trial, bidStatus, bidBody)
		}
		switch errorCode(t, bidBody) {
		case "AUCTION_EXPIRED", "AUCTION_NOT_ACTIVE", "AUCTION_NOT_FOUND":
		default:
			t.Fatalf("trial %d: bid error code = %q, want AUCTION_EXPIRED/AUCTION_NOT_ACTIVE/AUCTION_NOT_FOUND", trial, errorCode(t, bidBody))
		}
		if settleErr != nil {
			t.Fatalf("trial %d: SettleAuction() error = %v", trial, settleErr)
		}

		stored, err := auctionRepo.GetByID(ctx, testPool, auctionID)
		if err != nil {
			t.Fatalf("trial %d: GetByID() error = %v", trial, err)
		}
		if stored.Status != repository.AuctionExpired {
			t.Errorf("trial %d: auction status = %q, want EXPIRED", trial, stored.Status)
		}

		bids, err := bidRepo.ListByAuctionID(ctx, testPool, auctionID)
		if err != nil {
			t.Fatalf("trial %d: ListByAuctionID() error = %v", trial, err)
		}
		if len(bids) != 0 {
			t.Errorf("trial %d: bids = %+v, want none (the rejected bid must never have been persisted)", trial, bids)
		}

		bidderWallet := getWallet(t, bidder)
		if bidderWallet.ReservedBalance != 0 {
			t.Errorf("trial %d: bidder reserved_balance = %d, want 0 (no orphaned reservation)", trial, bidderWallet.ReservedBalance)
		}

		if _, err := invRepo.GetByGuildAndItem(ctx, testPool, owner, item.Item.ID); err != nil {
			t.Errorf("trial %d: seller inventory GetByGuildAndItem() error = %v, want item to remain with seller", trial, err)
		}
	}
}

func TestConcurrency_LastSecondBidVsSweep_BidExtendsSettlementSkips(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "LastSecond Owner", 0, 1000000)
	bidder := createGuildWithPouch(t, ctx, "LastSecond Bidder", 100000, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 1000, nil, &owner)
	auctionID := createAuctionFixture(t, ctx, item.Item.ID, owner, 1000, time.Now().UTC().Add(2*time.Second))

	var wg sync.WaitGroup
	var bidStatus int
	var bidBody []byte
	var bidErr, settleErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		bidStatus, bidBody, bidErr = doJSONRaw(http.MethodPost,
			fmt.Sprintf("/items/%d/bid", item.Item.ID), &bidder, map[string]any{"amount": 1000})
	}()
	go func() {
		defer wg.Done()
		settleErr = settler.SettleAuction(ctx, auctionID)
	}()
	wg.Wait()
	if bidErr != nil {
		t.Fatalf("bid request: %v", bidErr)
	}

	if bidStatus != http.StatusCreated {
		t.Fatalf("bid landing before end_time: status = %d, want 201; body = %s", bidStatus, bidBody)
	}
	if !errors.Is(settleErr, settlement.ErrAuctionNotEligible) {
		t.Fatalf("SettleAuction() error = %v, want ErrAuctionNotEligible (end_time was pushed out from under it)", settleErr)
	}

	stored, err := auctionRepo.GetByID(ctx, testPool, auctionID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.Status != repository.AuctionActive {
		t.Errorf("auction status = %q, want still ACTIVE (sweep must not force-settle a just-extended auction)", stored.Status)
	}
	if !stored.EndTime.After(time.Now().UTC()) {
		t.Errorf("end_time = %v, want extended into the future", stored.EndTime)
	}

	bids, err := bidRepo.ListByAuctionID(ctx, testPool, auctionID)
	if err != nil {
		t.Fatalf("ListByAuctionID() error = %v", err)
	}
	if len(bids) != 1 || bids[0].Status != repository.BidActive {
		t.Fatalf("bids = %+v, want exactly one ACTIVE bid", bids)
	}

	bidderWallet := getWallet(t, bidder)
	if bidderWallet.ReservedBalance != 1000 {
		t.Errorf("bidder reserved_balance = %d, want 1000 (bid accepted, not settled/released)", bidderWallet.ReservedBalance)
	}
}
