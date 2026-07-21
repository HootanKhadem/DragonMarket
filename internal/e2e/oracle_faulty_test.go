package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"DragonMarket/internal/oracle"
)

// TestOracle_FaultyResponses_ServesLastKnownGoodPrice is the ONLY test in
// this package that calls priceUpdater.Tick. That matters: Tick refreshes
// EVERY LEGENDARY item's cache entry in the DB in one call (it has no
// per-item filter), using each item's configured mock response (or the
// harness-wide default set once in TestMain if the item has no per-item
// override). If this test ran BEFORE some other test that later read a
// legendary item's live price via GET /items/{id}, that other test could
// observe a price clobbered by whatever this test configured. This is safe
// regardless of run order (including under -shuffle=on) because no OTHER
// test in this package reads back a legendary item's live price after
// creation -- an auction's base_price is captured once at creation time and
// never re-read live, and every other test that creates a LEGENDARY item
// only asserts on the price returned by the CREATE response itself.
//
// See harness_test.go's package comment and TestMain for why
// priceUpdater.PerItemTimeout is tightened to 300ms for this suite (so the
// "slow oracle" sub-test below induces its timeout in well under a second).
func TestOracle_FaultyResponses_ServesLastKnownGoodPrice(t *testing.T) {
	ctx := context.Background()
	item := createItemHTTP(t, ctx, "LEGENDARY", 5000, nil, nil)
	itemID := item.Item.ID

	// --- prime a good price ---
	mockOracle.SetForItem(itemID, oracle.MockResponse{Price: 6000, Jitter: 0})
	if err := priceUpdater.Tick(ctx); err != nil {
		t.Fatalf("priceUpdater.Tick() error = %v", err)
	}
	assertItemPrice(t, itemID, 6000)

	// --- invalid (zero) response: cache must keep the last known-good price ---
	mockOracle.SetForItem(itemID, oracle.MockResponse{Price: 0})
	if err := priceUpdater.Tick(ctx); err != nil {
		t.Fatalf("priceUpdater.Tick() error = %v", err)
	}
	assertItemPrice(t, itemID, 6000)

	// --- invalid (negative) response: same expectation ---
	mockOracle.SetForItem(itemID, oracle.MockResponse{Price: -50})
	if err := priceUpdater.Tick(ctx); err != nil {
		t.Fatalf("priceUpdater.Tick() error = %v", err)
	}
	assertItemPrice(t, itemID, 6000)

	// --- slow oracle: Delay exceeds priceUpdater.PerItemTimeout (300ms),
	// so refreshItem's per-item context times out; the tick must not error
	// out or hang the whole suite, and the cache must still hold 6000. ---
	mockOracle.SetForItem(itemID, oracle.MockResponse{Price: 7000, Delay: 800 * time.Millisecond})
	start := time.Now()
	if err := priceUpdater.Tick(ctx); err != nil {
		t.Fatalf("priceUpdater.Tick() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Tick() took %v, want bounded by PerItemTimeout (~300ms), not the oracle's full 800ms delay times item count", elapsed)
	}
	assertItemPrice(t, itemID, 6000)

	// --- recovery: once the oracle is healthy again, the next tick must
	// pick up the new price immediately. ---
	mockOracle.SetForItem(itemID, oracle.MockResponse{Price: 7000, Jitter: 0})
	if err := priceUpdater.Tick(ctx); err != nil {
		t.Fatalf("priceUpdater.Tick() error = %v", err)
	}
	assertItemPrice(t, itemID, 7000)
}

func assertItemPrice(t *testing.T, itemID int64, want int) {
	t.Helper()
	status, raw := doJSON(t, http.MethodGet, fmt.Sprintf("/items/%d", itemID), nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
	got := decodeInto[itemDTO](t, raw)
	if got.Price != want {
		t.Errorf("GET /items/%d price = %d, want %d", itemID, got.Price, want)
	}
}
