package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestDailyLimit_ExceededThenRecoversAfterSimulatedDayRollover(t *testing.T) {
	ctx := context.Background()

	const dailyLimit = 1000
	buyer := createGuildWithPouch(t, ctx, "DailyLimit Buyer", 50000, dailyLimit)

	qty := 10
	itemA := createItemHTTP(t, ctx, "COMMON", 900, &qty, nil) // spent_today -> 900
	itemB := createItemHTTP(t, ctx, "COMMON", 200, &qty, nil) // 900+200 = 1100 > 1000

	status, raw := doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/purchase", itemA.Item.ID), &buyer,
		map[string]any{"quantity": 1})
	requireStatus(t, status, http.StatusOK, raw)

	walletAfterA := getWallet(t, buyer)
	if walletAfterA.SpentToday != 900 {
		t.Fatalf("spent_today after buying item A = %d, want 900", walletAfterA.SpentToday)
	}

	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/purchase", itemB.Item.ID), &buyer,
		map[string]any{"quantity": 1})
	if status != http.StatusConflict {
		t.Fatalf("purchase over daily limit: status = %d, want 409; body = %s", status, raw)
	}
	if code := errorCode(t, raw); code != "DAILY_LIMIT_EXCEEDED" {
		t.Errorf("purchase over daily limit: error code = %q, want DAILY_LIMIT_EXCEEDED", code)
	}

	walletStillA := getWallet(t, buyer)
	if walletStillA.SpentToday != 900 {
		t.Errorf("spent_today after rejected purchase = %d, want unchanged 900", walletStillA.SpentToday)
	}

	if _, err := testPool.Exec(ctx,
		`UPDATE gold_pouches SET last_reset_date = last_reset_date - INTERVAL '1 day' WHERE guild_id = $1`,
		buyer,
	); err != nil {
		t.Fatalf("fixture: backdate last_reset_date: %v", err)
	}

	status, raw = doJSON(t, http.MethodPost, fmt.Sprintf("/items/%d/purchase", itemB.Item.ID), &buyer,
		map[string]any{"quantity": 1})
	requireStatus(t, status, http.StatusOK, raw)

	walletAfterReset := getWallet(t, buyer)
	if walletAfterReset.SpentToday != 200 {
		t.Errorf("spent_today after post-rollover purchase = %d, want 200 (reset then re-spent)", walletAfterReset.SpentToday)
	}
	if walletAfterReset.TotalBalance != 50000-900-200 {
		t.Errorf("total_balance = %d, want %d", walletAfterReset.TotalBalance, 50000-900-200)
	}
}
