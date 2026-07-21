package settlement

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"DragonMarket/internal/migrate"
	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

// This package's tests need a real shared *pgxpool.Pool (not a per-test
// nested transaction like internal/service uses for its non-concurrency
// tests): the concurrency test races AuctionService.PlaceBid against
// Settler.SettleAuction across two independent connections/transactions, and
// SettleAuction itself always opens its own transaction per auction (per the
// brief's "one locked transaction per auction, not one giant transaction for
// the whole sweep" design) -- so every test here runs directly against
// testPool rather than a rollback-at-cleanup tx.
func init() {
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_settlement_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		return 1
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		return 1
	}

	if err := migrate.Up(connStr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "migrate.Up() failed: %v\n", err)
		return 1
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to parse pool config: %v\n", err)
		return 1
	}
	poolCfg.MaxConns = 20

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to connect pgxpool: %v\n", err)
		return 1
	}
	defer pool.Close()
	testPool = pool

	return m.Run()
}

func newTestGoldPouchService() *service.GoldPouchService {
	return service.NewGoldPouchService(repository.NewGoldPouchRepository(), repository.NewTransactionLogRepository())
}

func newTestSettler() *Settler {
	return NewSettler(
		testPool,
		repository.NewAuctionRepository(),
		repository.NewBidRepository(),
		repository.NewInventoryRepository(),
		newTestGoldPouchService(),
		time.Second,
	)
}

func newTestAuctionService() *service.AuctionService {
	return service.NewAuctionService(
		testPool,
		repository.NewAuctionRepository(),
		repository.NewBidRepository(),
		repository.NewItemRepository(),
		repository.NewInventoryRepository(),
		newTestGoldPouchService(),
		oracle.NewCache(),
	)
}

func createTestGuild(t *testing.T, ctx context.Context, db repository.DBTX, name string) repository.Guild {
	t.Helper()
	leader, err := repository.NewCharacterRepository().Create(ctx, db, repository.Character{
		Name:         name + " Leader",
		LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create leader character: %v", err)
	}
	g, err := repository.NewGuildRepository().Create(ctx, db, repository.Guild{
		Name:              name,
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guild %q: %v", name, err)
	}
	return g
}

func createTestGoldPouch(t *testing.T, ctx context.Context, db repository.DBTX, gp repository.GoldPouch) repository.GoldPouch {
	t.Helper()
	if gp.LastResetDate.IsZero() {
		gp.LastResetDate = time.Now().UTC().Truncate(24 * time.Hour)
	}
	created, err := repository.NewGoldPouchRepository().Create(ctx, db, gp)
	if err != nil {
		t.Fatalf("fixture: create gold pouch for guild %d: %v", gp.GuildID, err)
	}
	return created
}

func createTestLegendaryItemOwnedBy(t *testing.T, ctx context.Context, db repository.DBTX, name string, price int, owner repository.Guild) repository.Item {
	t.Helper()
	forger, err := repository.NewCharacterRepository().Create(ctx, db, repository.Character{
		Name:         name + " Forger",
		LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create forger character: %v", err)
	}
	item, err := repository.NewItemRepository().Create(ctx, db, repository.Item{
		Name: name, LandOfOrigin: "Testland", Rarity: repository.RarityLegendary,
		ForgerCharacterID: forger.ID, Price: price,
	})
	if err != nil {
		t.Fatalf("fixture: create legendary item %q: %v", name, err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, db, repository.Inventory{
		GuildID: owner.ID, ItemID: item.ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("fixture: create inventory for item %q: %v", name, err)
	}
	return item
}

// createTestAuctionFixture creates an auction row directly (bypassing
// AuctionService.CreateAuction) so tests can set an end_time in the past --
// CreateAuction always computes end_time from "now", which can't produce an
// already-expired auction.
func createTestAuctionFixture(t *testing.T, ctx context.Context, db repository.DBTX, item repository.Item, owner repository.Guild, basePrice int, endTime time.Time) repository.Auction {
	t.Helper()
	endTime = endTime.UTC().Truncate(time.Microsecond)
	start := endTime.Add(-time.Hour)
	if now := time.Now().UTC().Truncate(time.Microsecond); start.After(now) {
		start = now
	}
	a, err := repository.NewAuctionRepository().Create(ctx, db, repository.Auction{
		ItemID: item.ID, OwnerGuildID: owner.ID, StartTime: start, EndTime: endTime, BasePrice: basePrice,
	})
	if err != nil {
		t.Fatalf("fixture: create auction for item %d: %v", item.ID, err)
	}
	return a
}
