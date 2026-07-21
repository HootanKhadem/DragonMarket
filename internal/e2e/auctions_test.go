package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestAuctions_HappyPath_CreateListGetBidCancelWallet(t *testing.T) {
	ctx := context.Background()
	itemID := itemIDByName(t, ctx, "Soul Reaver")
	owner := currentOwnerGuildID(t, ctx, itemID)
	bidder1 := createGuildWithPouch(t, ctx, "AuctionsHappyPath Bidder1", 1000000, 1000000)
	bidder2 := createGuildWithPouch(t, ctx, "AuctionsHappyPath Bidder2", 1000000, 1000000)

	// --- create ---
	status, raw := doJSON(t, http.MethodPost, "/auctions", &owner, map[string]any{
		"item_id": itemID, "duration_seconds": 3600,
	})
	requireStatus(t, status, http.StatusCreated, raw)
	auction := decodeInto[auctionDTO](t, raw)

	if auction.ItemID != itemID {
		t.Errorf("item_id = %d, want %d", auction.ItemID, itemID)
	}
	if auction.OwnerGuildID != owner {
		t.Errorf("owner_guild_id = %d, want %d", auction.OwnerGuildID, owner)
	}
	if auction.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", auction.Status)
	}
	if auction.BasePrice <= 0 {
		t.Errorf("base_price = %d, want > 0", auction.BasePrice)
	}
	if !auction.EndTime.After(auction.StartTime) {
		t.Errorf("end_time %v must be after start_time %v", auction.EndTime, auction.StartTime)
	}

	// --- list ---
	status, raw = doJSON(t, http.MethodGet, "/auctions?limit=500", nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	list := decodeInto[auctionListResponseDTO](t, raw)
	found := false
	for _, a := range list.Auctions {
		if a.ID == auction.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("GET /auctions did not include auction %d", auction.ID)
	}

	// --- get ---
	status, raw = doJSON(t, http.MethodGet, fmt.Sprintf("/auctions/%d", auction.ID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	got := decodeInto[auctionDTO](t, raw)
	if got.ID != auction.ID {
		t.Errorf("GET /auctions/{id} id = %d, want %d", got.ID, auction.ID)
	}

	// The OWNER trying to bid on its own auction must be rejected.
	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", itemID), &owner,
		map[string]any{"amount": auction.BasePrice})
	if status != http.StatusConflict {
		t.Fatalf("self-bid: status = %d, want 409; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "SELF_BID_NOT_ALLOWED" {
		t.Errorf("self-bid: error code = %q, want SELF_BID_NOT_ALLOWED", code)
	}

	// --- bid 1 (bidder1, at exactly base_price) ---
	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", itemID), &bidder1,
		map[string]any{"amount": auction.BasePrice})
	requireStatus(t, status, http.StatusCreated, raw)
	bid1 := decodeInto[bidDTO](t, raw)
	if bid1.Status != "ACTIVE" {
		t.Errorf("bid1 status = %q, want ACTIVE", bid1.Status)
	}

	// --- bid 2 (bidder2, comfortably above the 105% floor) ---
	bid2Amount := auction.BasePrice * 2
	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/bid", itemID), &bidder2,
		map[string]any{"amount": bid2Amount})
	requireStatus(t, status, http.StatusCreated, raw)
	bid2 := decodeInto[bidDTO](t, raw)

	// bidder1's reservation should now show up as reserved.
	w1 := getWallet(t, bidder1)
	if w1.ReservedBalance < auction.BasePrice {
		t.Errorf("bidder1 reserved_balance = %d, want >= %d", w1.ReservedBalance, auction.BasePrice)
	}

	// --- cancel bid1 (no longer the highest, so cancellation is allowed) ---
	status, raw = doJSON(t, http.MethodDelete,
		fmt.Sprintf("/items/%d/bid/%d", itemID, bid1.ID), &bidder1, nil)
	if status != http.StatusNoContent {
		t.Fatalf("cancel bid1: status = %d, want 204; body = %s", status, raw)
	}

	w1After := getWallet(t, bidder1)
	if w1After.ReservedBalance != 0 {
		t.Errorf("bidder1 reserved_balance after cancel = %d, want 0", w1After.ReservedBalance)
	}

	// Cancelling the current HIGHEST bid (bid2) must be rejected.
	status, raw = doJSON(t, http.MethodDelete,
		fmt.Sprintf("/items/%d/bid/%d", itemID, bid2.ID), &bidder2, nil)
	if status != http.StatusConflict {
		t.Fatalf("cancel highest bid: status = %d, want 409; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "BID_IS_HIGHEST" {
		t.Errorf("cancel highest bid: error code = %q, want BID_IS_HIGHEST", code)
	}

	// --- wallet check (bidder2 still holds its reservation) ---
	w2 := getWallet(t, bidder2)
	if w2.ReservedBalance != bid2Amount {
		t.Errorf("bidder2 reserved_balance = %d, want %d", w2.ReservedBalance, bid2Amount)
	}

	// --- settle, so this test leaves no dangling ACTIVE auction behind ---
	ownerBefore := getWallet(t, owner)
	pushAuctionEndTimeIntoPast(t, ctx, auction.ID)
	if err := settler.SettleAuction(ctx, auction.ID); err != nil {
		t.Fatalf("settler.SettleAuction() error = %v", err)
	}

	ownerAfter := getWallet(t, owner)
	if ownerAfter.TotalBalance != ownerBefore.TotalBalance+bid2Amount {
		t.Errorf("owner total_balance after settlement = %d, want %d", ownerAfter.TotalBalance, ownerBefore.TotalBalance+bid2Amount)
	}
	w2After := getWallet(t, bidder2)
	if w2After.ReservedBalance != 0 {
		t.Errorf("bidder2 (winner) reserved_balance after settlement = %d, want 0", w2After.ReservedBalance)
	}
	if _, err := invRepo.GetByGuildAndItem(ctx, testPool, bidder2, itemID); err != nil {
		t.Errorf("winner inventory GetByGuildAndItem() error = %v, want Soul Reaver to now belong to bidder2", err)
	}

	status, raw = doJSON(t, http.MethodGet, fmt.Sprintf("/auctions/%d", auction.ID), nil, nil)
	if status != http.StatusNotFound {
		t.Errorf("GET settled auction: status = %d, want 404; body = %s", status, raw)
	}
}
