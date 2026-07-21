package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// TestItems_Create_CommonAndRare_GetsActiveListing covers the happy path of
// POST /items for the two listable rarities: both must come back with a
// non-nil listing_id (the item is immediately purchasable).
func TestItems_Create_CommonAndRare_GetsActiveListing(t *testing.T) {
	ctx := context.Background()

	for _, rarity := range []string{"COMMON", "RARE"} {
		qty := 10
		created := createItemHTTP(t, ctx, rarity, 100, &qty, nil)

		if created.Item.Rarity != rarity {
			t.Errorf("rarity = %q, want %q", created.Item.Rarity, rarity)
		}
		if created.Item.IsLegendary {
			t.Errorf("%s item: is_legendary = true, want false", rarity)
		}
		if created.Item.AuctionOnly {
			t.Errorf("%s item: auction_only = true, want false", rarity)
		}
		if created.ListingID == nil {
			t.Fatalf("%s item: listing_id = nil, want a listing", rarity)
		}
		if created.Quantity != qty {
			t.Errorf("%s item: quantity = %d, want %d", rarity, created.Quantity, qty)
		}

		// Default guild (no guild_id in the request) must resolve to the
		// seeded "Vorynthax Guild".
		if created.GuildID != seededGuildID[guildVorynthax] {
			t.Errorf("%s item: guild_id = %d, want default guild %d", rarity, created.GuildID, seededGuildID[guildVorynthax])
		}
	}
}

// TestItems_Create_Legendary_NoListing proves LEGENDARY items get NO
// listing/auction on creation -- they only become sellable via the auction
// flow (Task 9/10), never via POST /items/{id}/purchase.
func TestItems_Create_Legendary_NoListing(t *testing.T) {
	ctx := context.Background()
	created := createItemHTTP(t, ctx, "LEGENDARY", 9000, nil, nil)

	if !created.Item.IsLegendary {
		t.Errorf("is_legendary = false, want true")
	}
	if !created.Item.AuctionOnly {
		t.Errorf("auction_only = false, want true")
	}
	if created.ListingID != nil {
		t.Errorf("listing_id = %d, want nil for a LEGENDARY item", *created.ListingID)
	}
	if created.Quantity != 1 {
		t.Errorf("quantity = %d, want 1 (legendaries are always unique)", created.Quantity)
	}

	// A legendary item can't be bought via the purchase endpoint.
	buyer := seededGuildID[guildFellowship]
	status, raw := doJSON(t, http.MethodPost,
		fmt.Sprintf("/items/%d/purchase", created.Item.ID), &buyer, map[string]any{"quantity": 1})
	if status != http.StatusConflict {
		t.Fatalf("purchase of a LEGENDARY item: status = %d, want 409; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "LISTING_NOT_ACTIVE" {
		t.Errorf("purchase of a LEGENDARY item: error code = %q, want LISTING_NOT_ACTIVE", code)
	}
}

// TestItems_List_And_Get exercises GET /items and GET /items/{id} against a
// freshly created item.
func TestItems_List_And_Get(t *testing.T) {
	ctx := context.Background()
	qty := 5
	created := createItemHTTP(t, ctx, "COMMON", 42, &qty, nil)

	status, raw := doJSON(t, http.MethodGet, fmt.Sprintf("/items/%d", created.Item.ID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	got := decodeInto[itemDTO](t, raw)
	if got.ID != created.Item.ID || got.Name != created.Item.Name || got.Price != 42 {
		t.Errorf("GET /items/{id} = %+v, want to match created item %+v", got, created.Item)
	}

	status, raw = doJSON(t, http.MethodGet, "/items?limit=500", nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	list := decodeInto[itemListResponseDTO](t, raw)
	found := false
	for _, it := range list.Items {
		if it.ID == created.Item.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GET /items did not include newly created item %d", created.Item.ID)
	}

	// Unknown item ID.
	status, raw = doJSON(t, http.MethodGet, "/items/999999999", nil, nil)
	if status != http.StatusNotFound {
		t.Errorf("GET /items/{unknown}: status = %d, want 404; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "ITEM_NOT_FOUND" {
		t.Errorf("GET /items/{unknown}: error code = %q, want ITEM_NOT_FOUND", code)
	}
}

// TestItems_Purchase_SeededCommonItem_HappyPath purchases one of the seeded
// COMMON items end to end and checks both guilds' wallets moved by exactly
// the right amount. This test claims "Traveler's Cloak" (seeded COMMON,
// qty 20, owned by Camelot's Round Table, price 10) exclusively -- no other
// test in this package touches that seeded row.
func TestItems_Purchase_SeededCommonItem_HappyPath(t *testing.T) {
	ctx := context.Background()
	itemID := itemIDByName(t, ctx, "Traveler's Cloak")
	buyer := seededGuildID[guildFellowship]
	seller := seededGuildID[guildCamelot]

	buyerBefore := getWallet(t, buyer)
	sellerBefore := getWallet(t, seller)

	status, raw := doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/purchase", itemID), &buyer,
		map[string]any{"quantity": 1})
	requireStatus(t, status, http.StatusOK, raw)
	purchase := decodeInto[purchaseResponseDTO](t, raw)

	if purchase.UnitPrice != 10 {
		t.Errorf("unit_price = %d, want 10 (seeded price)", purchase.UnitPrice)
	}
	if purchase.TotalPrice != 10 {
		t.Errorf("total_price = %d, want 10", purchase.TotalPrice)
	}
	if purchase.SellerGuildID != seller {
		t.Errorf("seller_guild_id = %d, want %d", purchase.SellerGuildID, seller)
	}
	if purchase.ListingStatus != "ACTIVE" {
		t.Errorf("listing_status = %q, want ACTIVE (qty 20 -> 19, still active)", purchase.ListingStatus)
	}

	buyerAfter := getWallet(t, buyer)
	sellerAfter := getWallet(t, seller)

	if buyerAfter.TotalBalance != buyerBefore.TotalBalance-10 {
		t.Errorf("buyer total_balance = %d, want %d", buyerAfter.TotalBalance, buyerBefore.TotalBalance-10)
	}
	if buyerAfter.ReservedBalance != buyerBefore.ReservedBalance {
		t.Errorf("buyer reserved_balance = %d, want unchanged at %d (settled, not left reserved)", buyerAfter.ReservedBalance, buyerBefore.ReservedBalance)
	}
	if sellerAfter.TotalBalance != sellerBefore.TotalBalance+10 {
		t.Errorf("seller total_balance = %d, want %d", sellerAfter.TotalBalance, sellerBefore.TotalBalance+10)
	}
}

// getWallet is a small typed wrapper around GET /guilds/{id}/wallet used
// throughout this package.
func getWallet(t *testing.T, guildID int64) walletDTO {
	t.Helper()
	status, raw := doJSON(t, http.MethodGet, fmt.Sprintf("/guilds/%d/wallet", guildID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	return decodeInto[walletDTO](t, raw)
}
