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

func newTestService() *GoldPouchService {
	return NewGoldPouchService(repository.NewGoldPouchRepository(), repository.NewTransactionLogRepository())
}

func TestGoldPouchService_ReserveThenRelease_RestoresUsableBalanceExactly(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Reserve Release Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		DailySpendingLimit: 500,
	})
	svc := newTestService()
	pouchRepo := repository.NewGoldPouchRepository()

	before, err := pouchRepo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}

	if err := svc.Reserve(ctx, tx, guild.ID, 400, new("listing:1")); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	mid, err := pouchRepo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if mid.ReservedBalance != 400 || mid.UsableBalance != 600 {
		t.Fatalf("after Reserve() = %+v, want ReservedBalance=400 UsableBalance=600", mid)
	}

	if err := svc.Release(ctx, tx, guild.ID, 400, new("listing:1")); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	after, err := pouchRepo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if after.ReservedBalance != 0 || after.UsableBalance != before.UsableBalance {
		t.Fatalf("after Release() = %+v, want ReservedBalance=0 UsableBalance=%d", after, before.UsableBalance)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("ListByGuildID() len = %d, want 2", len(logs))
	}
	if logs[0].Type != repository.TxReserve || logs[0].Amount != 400 || logs[0].Reference == nil || *logs[0].Reference != "listing:1" {
		t.Errorf("logs[0] = %+v, want Type=RESERVE Amount=400 Reference=listing:1", logs[0])
	}
	if logs[1].Type != repository.TxRelease || logs[1].Amount != 400 {
		t.Errorf("logs[1] = %+v, want Type=RELEASE Amount=400", logs[1])
	}
}

func TestGoldPouchService_Reserve_RejectsWhenUsableBalanceInsufficient(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Reserve Insufficient Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       100,
		DailySpendingLimit: 500,
	})
	svc := newTestService()

	err := svc.Reserve(ctx, tx, guild.ID, 150, nil)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("Reserve() error = %v, want ErrInsufficientBalance", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.ReservedBalance != 0 {
		t.Errorf("ReservedBalance = %d, want 0 (rejected reserve must not mutate state)", pouch.ReservedBalance)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListByGuildID() len = %d, want 0 (no log on rejected reserve)", len(logs))
	}
}

func TestGoldPouchService_Release_RejectsWhenReservedBalanceInsufficient(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Release Insufficient Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    50,
		DailySpendingLimit: 500,
	})
	svc := newTestService()

	err := svc.Release(ctx, tx, guild.ID, 100, nil)
	if !errors.Is(err, ErrInsufficientReserved) {
		t.Fatalf("Release() error = %v, want ErrInsufficientReserved", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.ReservedBalance != 50 {
		t.Errorf("ReservedBalance = %d, want 50 (rejected release must not mutate state)", pouch.ReservedBalance)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListByGuildID() len = %d, want 0 (no log on rejected release)", len(logs))
	}
}

func TestGoldPouchService_ReserveThenSettle_MovesFundsAndIncrementsSpentToday(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Reserve Settle Guild")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		DailySpendingLimit: 500,
		LastResetDate:      today,
	})
	svc := newTestService()
	pouchRepo := repository.NewGoldPouchRepository()

	if err := svc.Reserve(ctx, tx, guild.ID, 300, new("listing:7")); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := svc.Settle(ctx, tx, guild.ID, 300, repository.TxPurchase, new("listing:7")); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}

	after, err := pouchRepo.GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if after.TotalBalance != 700 || after.ReservedBalance != 0 || after.SpentToday != 300 || after.UsableBalance != 700 {
		t.Fatalf("after Settle() = %+v, want TotalBalance=700 ReservedBalance=0 SpentToday=300 UsableBalance=700", after)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 2 || logs[1].Type != repository.TxPurchase || logs[1].Amount != 300 {
		t.Fatalf("logs = %+v, want [.., {Type:PURCHASE Amount:300}]", logs)
	}
}

func TestGoldPouchService_Settle_RejectsWhenExceedsDailyLimit(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Settle DailyLimit Guild")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 100,
		SpentToday:         80,
		LastResetDate:      today,
	})
	svc := newTestService()

	err := svc.Settle(ctx, tx, guild.ID, 30, repository.TxPurchase, nil)
	if !errors.Is(err, ErrDailyLimitExceeded) {
		t.Fatalf("Settle() error = %v, want ErrDailyLimitExceeded", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.TotalBalance != 1000 || pouch.ReservedBalance != 200 || pouch.SpentToday != 80 {
		t.Errorf("state after rejected Settle() = %+v, want unchanged (Total=1000 Reserved=200 SpentToday=80)", pouch)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListByGuildID() len = %d, want 0 (no log on rejected settle)", len(logs))
	}
}

func TestGoldPouchService_Settle_RejectsWhenReservedBalanceInsufficient(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Settle Insufficient Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    20,
		DailySpendingLimit: 100000,
	})
	svc := newTestService()

	err := svc.Settle(ctx, tx, guild.ID, 50, repository.TxPurchase, nil)
	if !errors.Is(err, ErrInsufficientReserved) {
		t.Fatalf("Settle() error = %v, want ErrInsufficientReserved", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.TotalBalance != 1000 || pouch.ReservedBalance != 20 {
		t.Errorf("state after rejected Settle() = %+v, want unchanged (Total=1000 Reserved=20)", pouch)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListByGuildID() len = %d, want 0 (no log on rejected settle)", len(logs))
	}
}

func TestGoldPouchService_Settle_ResetsSpentTodayAcrossUTCDayBoundary(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Settle DayBoundary Guild")
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 100,
		SpentToday:         90, // would reject a 50-gold settle if not reset first (90+50 > 100)
		LastResetDate:      yesterday,
	})
	svc := newTestService()

	if err := svc.Settle(ctx, tx, guild.ID, 50, repository.TxPurchase, nil); err != nil {
		t.Fatalf("Settle() error = %v, want nil (lazy reset should have zeroed SpentToday first)", err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.SpentToday != 50 {
		t.Errorf("SpentToday = %d, want 50 (reset to 0 then +50)", pouch.SpentToday)
	}
	if !pouch.LastResetDate.UTC().Equal(today) {
		t.Errorf("LastResetDate = %v, want %v", pouch.LastResetDate.UTC(), today)
	}
	if pouch.TotalBalance != 950 || pouch.ReservedBalance != 150 {
		t.Errorf("pouch = %+v, want TotalBalance=950 ReservedBalance=150", pouch)
	}
}

func TestGoldPouchService_Settle_RejectsInvalidTransactionType(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Settle InvalidType Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 100000,
	})
	svc := newTestService()

	err := svc.Settle(ctx, tx, guild.ID, 50, repository.TxReserve, nil)
	if !errors.Is(err, ErrInvalidTransactionType) {
		t.Fatalf("Settle() error = %v, want ErrInvalidTransactionType", err)
	}
}

func TestGoldPouchService_Settle_AcceptsAuctionWin(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Settle AuctionWin Guild")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    200,
		DailySpendingLimit: 100000,
		LastResetDate:      today,
	})
	svc := newTestService()

	if err := svc.Settle(ctx, tx, guild.ID, 50, repository.TxAuctionWin, new("auction:4")); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 1 || logs[0].Type != repository.TxAuctionWin || *logs[0].Reference != "auction:4" {
		t.Fatalf("logs = %+v, want [{Type:AUCTION_WIN Reference:auction:4}]", logs)
	}
}

func TestGoldPouchService_Credit_IncrementsTotalBalanceOnly(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Credit Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       500,
		ReservedBalance:    100,
		DailySpendingLimit: 100,
		SpentToday:         40,
	})
	svc := newTestService()

	if err := svc.Credit(ctx, tx, guild.ID, 200, new("auction:9")); err != nil {
		t.Fatalf("Credit() error = %v", err)
	}

	pouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if pouch.TotalBalance != 700 || pouch.ReservedBalance != 100 || pouch.SpentToday != 40 || pouch.UsableBalance != 600 {
		t.Fatalf("after Credit() = %+v, want TotalBalance=700 ReservedBalance=100 SpentToday=40 UsableBalance=600", pouch)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != 1 || logs[0].Type != repository.TxCredit || logs[0].Amount != 200 || *logs[0].Reference != "auction:9" {
		t.Fatalf("logs = %+v, want [{Type:CREDIT Amount:200 Reference:auction:9}]", logs)
	}
}

func TestGoldPouchService_RejectsNonPositiveAmount(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Invalid Amount Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       1000,
		ReservedBalance:    100,
		DailySpendingLimit: 1000,
	})
	svc := newTestService()

	cases := []struct {
		name string
		call func() error
	}{
		{"Reserve/zero", func() error { return svc.Reserve(ctx, tx, guild.ID, 0, nil) }},
		{"Reserve/negative", func() error { return svc.Reserve(ctx, tx, guild.ID, -5, nil) }},
		{"Release/zero", func() error { return svc.Release(ctx, tx, guild.ID, 0, nil) }},
		{"Settle/zero", func() error { return svc.Settle(ctx, tx, guild.ID, 0, repository.TxPurchase, nil) }},
		{"Credit/zero", func() error { return svc.Credit(ctx, tx, guild.ID, 0, nil) }},
		{"Credit/negative", func() error { return svc.Credit(ctx, tx, guild.ID, -1, nil) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(); !errors.Is(err, ErrInvalidAmount) {
				t.Fatalf("error = %v, want ErrInvalidAmount", err)
			}
		})
	}
}

func TestGoldPouchService_Reserve_PropagatesNotFoundForMissingPouch(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	svc := newTestService()

	err := svc.Reserve(ctx, tx, 999999999, 10, nil)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Reserve() error = %v, want repository.ErrNotFound", err)
	}
}

// TestGoldPouchService_Reserve_ConcurrentReservationsSerializeCorrectly is
// the concurrency test mandated by the brief: N goroutines, each opening
// its OWN transaction against the shared pool (not one shared tx -- this
// is testing that separate concurrent transactions serialize via
// SELECT ... FOR UPDATE, not that one transaction is internally
// consistent), race to reserve funds from the same gold_pouch where the
// sum of all attempts exceeds total_balance. With equal per-goroutine
// amounts the number of possible winners is deterministic regardless of
// scheduling order, so we can assert an exact count.
func TestGoldPouchService_Reserve_ConcurrentReservationsSerializeCorrectly(t *testing.T) {
	ctx := context.Background()
	// Fixture is committed directly against testPool (see createTestGuild's
	// doc comment for why), so unlike every other test in this file it is
	// NOT cleaned up by a beginTx(t) rollback -- it persists in the shared
	// container for the rest of the process. A hardcoded name would then
	// collide with guilds.name's UNIQUE constraint on any repeat run in the
	// same process (e.g. `go test -count=3`), so the name must be unique
	// per invocation.
	guildName := fmt.Sprintf("Concurrent Reserve Guild %d", time.Now().UnixNano())
	guild := createTestGuild(t, ctx, testPool, guildName)
	const totalBalance = 1000
	const amount = 150
	const n = 10                                // 10 * 150 = 1500 > 1000: not all can succeed
	const wantSuccesses = totalBalance / amount // = 6

	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{
		GuildID:            guild.ID,
		TotalBalance:       totalBalance,
		DailySpendingLimit: 1000000,
	})
	svc := newTestService()

	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gtx, err := testPool.Begin(ctx)
			if err != nil {
				results[i] = fmt.Errorf("begin: %w", err)
				return
			}
			reserveErr := svc.Reserve(ctx, gtx, guild.ID, amount, new(fmt.Sprintf("goroutine:%d", i)))
			if reserveErr != nil {
				_ = gtx.Rollback(ctx)
				results[i] = reserveErr
				return
			}
			if commitErr := gtx.Commit(ctx); commitErr != nil {
				results[i] = fmt.Errorf("commit: %w", commitErr)
			}
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInsufficientBalance):
			failures++
		default:
			t.Fatalf("goroutine returned unexpected error: %v", err)
		}
	}

	if successes != wantSuccesses {
		t.Errorf("successes = %d, want %d", successes, wantSuccesses)
	}
	if failures != n-wantSuccesses {
		t.Errorf("failures = %d, want %d", failures, n-wantSuccesses)
	}

	final, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, testPool, guild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID() error = %v", err)
	}
	if final.ReservedBalance != wantSuccesses*amount {
		t.Errorf("final ReservedBalance = %d, want %d", final.ReservedBalance, wantSuccesses*amount)
	}
	if final.UsableBalance != totalBalance-wantSuccesses*amount {
		t.Errorf("final UsableBalance = %d, want %d", final.UsableBalance, totalBalance-wantSuccesses*amount)
	}

	logs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, testPool, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(logs) != wantSuccesses {
		t.Errorf("transaction_logs len = %d, want %d (only successful reserves log)", len(logs), wantSuccesses)
	}
}
