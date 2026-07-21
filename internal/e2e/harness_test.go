package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"DragonMarket/internal/api"
	"DragonMarket/internal/migrate"
	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
	"DragonMarket/internal/settlement"
)

func init() {
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

const (
	guildVorynthax  = "Vorynthax Guild"
	guildFellowship = "Fellowship of the Grey"
	guildIllidari   = "Illidari Vanguard"
	guildCamelot    = "Camelot's Round Table"
	guildWinterfell = "Winterfell Wardens"
)

var (
	testPool   *pgxpool.Pool
	testServer *httptest.Server

	itemRepo      repository.ItemRepository
	listingRepo   repository.ListingRepository
	invRepo       repository.InventoryRepository
	guildRepo     repository.GuildRepository
	auctionRepo   repository.AuctionRepository
	bidRepo       repository.BidRepository
	goldPouchRepo repository.GoldPouchRepository
	goldPouchSvc  *service.GoldPouchService

	mockOracle   *oracle.MockPriceOracleService
	priceCache   *oracle.Cache
	priceUpdater *oracle.Updater
	settler      *settlement.Settler

	seededGuildID map[string]int64
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_e2e_test"),
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
	poolCfg.MaxConns = 30

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to connect pgxpool: %v\n", err)
		return 1
	}
	defer pool.Close()
	testPool = pool

	if err := validateSeedData(ctx, pool); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "seed data validation failed: %v\n", err)
		return 1
	}

	itemRepo = repository.NewItemRepository()
	listingRepo = repository.NewListingRepository()
	invRepo = repository.NewInventoryRepository()
	guildRepo = repository.NewGuildRepository()
	auctionRepo = repository.NewAuctionRepository()
	bidRepo = repository.NewBidRepository()
	goldPouchRepo = repository.NewGoldPouchRepository()
	goldPouchSvc = service.NewGoldPouchService(goldPouchRepo, repository.NewTransactionLogRepository())

	mockOracle = oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 8000, Jitter: 0})

	priceCache = oracle.NewCache()
	priceUpdater = oracle.NewUpdater(mockOracle, priceCache, itemRepo, pool, time.Hour)
	priceUpdater.PerItemTimeout = 300 * time.Millisecond

	settler = settlement.NewSettler(pool, auctionRepo, bidRepo, invRepo, goldPouchSvc, time.Hour)

	router := api.NewRouter(api.Dependencies{
		Pool:          pool,
		Items:         itemRepo,
		Listings:      listingRepo,
		Inventories:   invRepo,
		Guilds:        guildRepo,
		Auctions:      auctionRepo,
		Bids:          bidRepo,
		GoldPouches:   goldPouchSvc,
		GoldPouchRepo: goldPouchRepo,
		PriceCache:    priceCache,
	})
	testServer = httptest.NewServer(router)
	defer testServer.Close()

	seededGuildID = make(map[string]int64)
	for _, name := range []string{guildVorynthax, guildFellowship, guildIllidari, guildCamelot, guildWinterfell} {
		g, err := guildRepo.GetByName(ctx, pool, name)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to resolve seeded guild %q: %v\n", name, err)
			return 1
		}
		seededGuildID[name] = g.ID
	}

	return m.Run()
}

func validateSeedData(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT rarity, COUNT(*) FROM items GROUP BY rarity`)
	if err != nil {
		return fmt.Errorf("query item rarity counts: %w", err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var rarity string
		var count int
		if err := rows.Scan(&rarity, &count); err != nil {
			rows.Close()
			return fmt.Errorf("scan item rarity count: %w", err)
		}
		counts[rarity] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate item rarity counts: %w", err)
	}
	rows.Close()

	for rarity, want := range map[string]int{"COMMON": 20, "RARE": 7, "LEGENDARY": 3} {
		if counts[rarity] != want {
			return fmt.Errorf("seeded %s items = %d, want %d (migrations/000011_seed_data drifted or failed)",
				rarity, counts[rarity], want)
		}
	}

	limitRows, err := pool.Query(ctx,
		`SELECT g.name, gp.total_balance, gp.daily_spending_limit FROM gold_pouches gp JOIN guilds g ON g.id = gp.guild_id`)
	if err != nil {
		return fmt.Errorf("query seeded gold pouches: %w", err)
	}
	defer limitRows.Close()

	found := 0
	for limitRows.Next() {
		var name string
		var totalBalance, dailyLimit int
		if err := limitRows.Scan(&name, &totalBalance, &dailyLimit); err != nil {
			return fmt.Errorf("scan seeded gold pouch: %w", err)
		}
		found++
		if totalBalance != 50000 {
			return fmt.Errorf("seeded guild %q total_balance = %d, want 50000", name, totalBalance)
		}
		if dailyLimit < 2000 || dailyLimit > 10000 {
			return fmt.Errorf("seeded guild %q daily_spending_limit = %d, want in [2000,10000]", name, dailyLimit)
		}
	}
	if err := limitRows.Err(); err != nil {
		return fmt.Errorf("iterate seeded gold pouches: %w", err)
	}
	if found != 5 {
		return fmt.Errorf("found %d seeded gold pouches, want 5 (one per seeded guild)", found)
	}
	return nil
}

func apiURL(path string) string { return testServer.URL + path }

func doJSONRaw(method, path string, guildID *int64, body any) (status int, respBody []byte, err error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, apiURL(path), reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if guildID != nil {
		req.Header.Set("X-Guild-ID", strconv.FormatInt(*guildID, 10))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("do request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response body for %s %s: %w", method, path, err)
	}
	return resp.StatusCode, respBody, nil
}

func doJSON(t *testing.T, method, path string, guildID *int64, body any) (int, []byte) {
	t.Helper()
	status, raw, err := doJSONRaw(method, path, guildID, body)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return status, raw
}

func decodeInto[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var out T
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response body %s: %v", raw, err)
	}
	return out
}

func requireStatus(t *testing.T, got, want int, raw []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d; body = %s", got, want, raw)
	}
}

func errorCode(t *testing.T, raw []byte) string {
	t.Helper()
	e := decodeInto[errorDTO](t, raw)
	return e.Error.Code
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

func createGuildWithPouch(t *testing.T, ctx context.Context, namePrefix string, totalBalance, dailyLimit int) int64 {
	t.Helper()
	name := uniqueName(namePrefix)

	leader, err := repository.NewCharacterRepository().Create(ctx, testPool, repository.Character{
		Name: name + " Leader", LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create leader character for guild %q: %v", name, err)
	}

	g, err := guildRepo.Create(ctx, testPool, repository.Guild{
		Name: name, LeaderCharacterID: leader.ID, LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guild %q: %v", name, err)
	}

	if _, err := goldPouchRepo.Create(ctx, testPool, repository.GoldPouch{
		GuildID: g.ID, TotalBalance: totalBalance, DailySpendingLimit: dailyLimit,
		LastResetDate: time.Now().UTC().Truncate(24 * time.Hour),
	}); err != nil {
		t.Fatalf("fixture: create gold pouch for guild %q: %v", name, err)
	}

	return g.ID
}

func pushAuctionEndTimeIntoPast(t *testing.T, ctx context.Context, auctionID int64) {
	t.Helper()
	a, err := auctionRepo.GetByID(ctx, testPool, auctionID)
	if err != nil {
		t.Fatalf("fixture: get auction %d: %v", auctionID, err)
	}
	a.EndTime = a.StartTime.Add(time.Millisecond)
	if err := auctionRepo.Update(ctx, testPool, a); err != nil {
		t.Fatalf("fixture: push end_time into the past for auction %d: %v", auctionID, err)
	}
}

func currentOwnerGuildID(t *testing.T, ctx context.Context, itemID int64) int64 {
	t.Helper()
	var guildID int64
	if err := testPool.QueryRow(ctx,
		`SELECT guild_id FROM inventories WHERE item_id = $1`, itemID,
	).Scan(&guildID); err != nil {
		t.Fatalf("fixture: query current owner of item %d: %v", itemID, err)
	}
	return guildID
}

func firstCharacterID(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var id int64
	if err := testPool.QueryRow(ctx, `SELECT id FROM characters ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("fixture: query first character: %v", err)
	}
	return id
}

func itemIDByName(t *testing.T, ctx context.Context, name string) int64 {
	t.Helper()
	var id int64
	if err := testPool.QueryRow(ctx, `SELECT id FROM items WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("fixture: query item %q: %v", name, err)
	}
	return id
}

func createItemHTTP(t *testing.T, ctx context.Context, rarity string, price int, quantity *int, guildID *int64) createItemResponseDTO {
	t.Helper()

	body := map[string]any{
		"name":                uniqueName(rarity + " Item"),
		"land_of_origin":      "Testland",
		"forger_character_id": firstCharacterID(t, ctx),
		"rarity":              rarity,
		"price":               price,
	}
	if quantity != nil {
		body["quantity"] = *quantity
	}
	if guildID != nil {
		body["guild_id"] = *guildID
	}

	status, raw := doJSON(t, http.MethodPost, "/items", nil, body)
	requireStatus(t, status, http.StatusCreated, raw)
	return decodeInto[createItemResponseDTO](t, raw)
}

func createAuctionFixture(t *testing.T, ctx context.Context, itemID, ownerGuildID int64, basePrice int, endTime time.Time) int64 {
	t.Helper()
	endTime = endTime.UTC().Truncate(time.Microsecond)
	start := endTime.Add(-time.Hour)
	if now := time.Now().UTC().Truncate(time.Microsecond); start.After(now) {
		start = now
	}
	a, err := auctionRepo.Create(ctx, testPool, repository.Auction{
		ItemID: itemID, OwnerGuildID: ownerGuildID, StartTime: start, EndTime: endTime, BasePrice: basePrice,
	})
	if err != nil {
		t.Fatalf("fixture: create auction for item %d: %v", itemID, err)
	}
	return a.ID
}
