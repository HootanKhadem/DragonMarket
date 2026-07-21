package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAuctionLifecycle_BidWithinLast5Minutes_ExtendsEndTime(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "Ext Owner", 0, 1000000)
	bidder := createGuildWithPouch(t, ctx, "Ext Bidder", 100000, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 1000, nil, &owner)

	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 30,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	auction := decodeInto[auctionDTO](t, raw)

	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", item.Item.ID), &bidder,
		map[string]any{"amount": auction.BasePrice})
	requireStatus(t, status, http.StatusCreated, raw)

	status, raw = doJSON(t, http.MethodGet, fmt.Sprintf("/auctions/%d", auction.ID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	after := decodeInto[auctionDTO](t, raw)

	if !after.EndTime.After(auction.EndTime) {
		t.Fatalf("end_time after bid = %v, want strictly after original end_time %v", after.EndTime, auction.EndTime)
	}
	extension := after.EndTime.Sub(auction.EndTime)
	if extension < 4*time.Minute+30*time.Second {
		t.Errorf("end_time extension = %v, want ~5 minutes", extension)
	}
}

func TestAuctionLifecycle_BidRejected_PastEndTime_BeforeSweep(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "PreSweep Owner", 0, 1000000)
	bidder := createGuildWithPouch(t, ctx, "PreSweep Bidder", 100000, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 1000, nil, &owner)

	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 2,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	auction := decodeInto[auctionDTO](t, raw)

	time.Sleep(2500 * time.Millisecond)

	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", item.Item.ID), &bidder,
		map[string]any{"amount": auction.BasePrice})
	if status != http.StatusConflict {
		t.Fatalf("bid past end_time (unswept): status = %d, want 409; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "AUCTION_EXPIRED" {
		t.Errorf("bid past end_time (unswept): error code = %q, want AUCTION_EXPIRED", code)
	}

	status, raw = doJSON(t, http.MethodGet, fmt.Sprintf("/auctions/%d", auction.ID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	stillActive := decodeInto[auctionDTO](t, raw)
	if stillActive.Status != "ACTIVE" {
		t.Errorf("auction status pre-sweep = %q, want still ACTIVE (proves rejection wasn't due to a status flip)", stillActive.Status)
	}
	if !time.Now().UTC().After(stillActive.EndTime) {
		t.Errorf("end_time = %v, want already in the past", stillActive.EndTime)
	}
}

func TestAuctionLifecycle_NoBidExpiry_FreesItemForFreshAuction(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "NoBid Owner", 0, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 1500, nil, &owner)

	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 2,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	firstAuction := decodeInto[auctionDTO](t, raw)

	time.Sleep(2500 * time.Millisecond)

	if err := settler.Tick(ctx); err != nil {
		t.Fatalf("settler.Tick() error = %v", err)
	}

	// The expired, no-bid auction must now read as gone.
	status, raw = doJSON(t, http.MethodGet, fmt.Sprintf("/auctions/%d", firstAuction.ID), nil, nil)
	if status != http.StatusNotFound {
		t.Errorf("GET swept no-bid auction: status = %d, want 404; body = %s", status, raw)
	}

	status, raw = doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 3600,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	second := decodeInto[auctionDTO](t, raw)
	if second.ID == firstAuction.ID {
		t.Errorf("second auction id = %d, want a new row distinct from %d", second.ID, firstAuction.ID)
	}
	if second.Status != "ACTIVE" {
		t.Errorf("second auction status = %q, want ACTIVE", second.Status)
	}
}

func TestAuctionLifecycle_WonAuction_TransfersItemAndSettlesFunds(t *testing.T) {
	ctx := context.Background()
	owner := createGuildWithPouch(t, ctx, "Won Owner", 0, 1000000)
	winner := createGuildWithPouch(t, ctx, "Won Winner", 100000, 1000000)
	item := createItemHTTP(t, ctx, "LEGENDARY", 2000, nil, &owner)

	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 3600,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	auction := decodeInto[auctionDTO](t, raw)

	const bidAmount = 2000
	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", item.Item.ID), &winner,
		map[string]any{"amount": bidAmount})
	requireStatus(t, status, http.StatusCreated, raw)

	ownerBefore := getWallet(t, owner)
	winnerBefore := getWallet(t, winner)
	if winnerBefore.ReservedBalance != bidAmount {
		t.Fatalf("winner reserved_balance before sweep = %d, want %d", winnerBefore.ReservedBalance, bidAmount)
	}

	pushAuctionEndTimeIntoPast(t, ctx, auction.ID)
	if err := settler.SettleAuction(ctx, auction.ID); err != nil {
		t.Fatalf("settler.SettleAuction() error = %v", err)
	}

	ownerAfter := getWallet(t, owner)
	winnerAfter := getWallet(t, winner)

	if ownerAfter.TotalBalance != ownerBefore.TotalBalance+bidAmount {
		t.Errorf("owner total_balance after settlement = %d, want %d", ownerAfter.TotalBalance, ownerBefore.TotalBalance+bidAmount)
	}
	if winnerAfter.TotalBalance != winnerBefore.TotalBalance-bidAmount {
		t.Errorf("winner total_balance after settlement = %d, want %d", winnerAfter.TotalBalance, winnerBefore.TotalBalance-bidAmount)
	}
	if winnerAfter.ReservedBalance != 0 {
		t.Errorf("winner reserved_balance after settlement = %d, want 0", winnerAfter.ReservedBalance)
	}

	if _, err := invRepo.GetByGuildAndItem(ctx, testPool, owner, item.Item.ID); err == nil {
		t.Errorf("seller inventory for item %d still exists after losing the auction", item.Item.ID)
	}
	if _, err := invRepo.GetByGuildAndItem(ctx, testPool, winner, item.Item.ID); err != nil {
		t.Errorf("winner inventory GetByGuildAndItem() error = %v, want the item to now belong to the winner", err)
	}

	status, raw = doJSON(t, http.MethodPost, "/auctions", &winner, map[string]any{
		"item_id": item.Item.ID, "duration_seconds": 3600,
	})
	requireStatus(t, status, http.StatusCreated, raw)
}
