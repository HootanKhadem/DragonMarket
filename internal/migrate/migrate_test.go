package migrate

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func init() {
	// On Windows + Docker Desktop, testcontainers-go's "ryuk" reaper sidecar
	// times out waiting to become ready (it resolves the docker socket over
	// npipe, and its own readiness probe doesn't reliably see it), which
	// stalls every test for ~60s before failing. Our tests already terminate
	// their own containers via t.Cleanup, so the reaper's automatic
	// cleanup-on-crash safety net isn't required for these tests to be
	// correct. Gated to windows specifically (rather than disabling ryuk
	// unconditionally for every OS/CI runner) so this workaround only kicks
	// in on the environment that actually needs it.
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// wantTables is the set of tables Task 3's migrations must create, each with
// a column that must exist on it.
var wantTables = map[string]string{
	"characters":       "guild_id",
	"guilds":           "leader_character_id",
	"items":            "rarity",
	"inventories":      "quantity",
	"gold_pouches":     "usable_balance",
	"listings":         "status",
	"auctions":         "end_time",
	"bids":             "amount",
	"transaction_logs": "reference",
}

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return connStr
}

func TestUp_CreatesExpectedTablesAndColumns(t *testing.T) {
	databaseURL := startPostgres(t)

	if err := Up(databaseURL); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	for table, column := range wantTables {
		var exists bool
		err := db.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2
			)`,
			table, column,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("query for %s.%s: %v", table, column, err)
		}
		if !exists {
			t.Errorf("expected column %s.%s to exist after Up(), it does not", table, column)
		}
	}
}

func TestUp_ThenDown_DropsAllTables(t *testing.T) {
	databaseURL := startPostgres(t)

	if err := Up(databaseURL); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if err := Down(databaseURL); err != nil {
		t.Fatalf("Down() error = %v", err)
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	for table := range wantTables {
		var exists bool
		err := db.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = $1
			)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("query for table %s: %v", table, err)
		}
		if exists {
			t.Errorf("expected table %s to be dropped after Down(), it still exists", table)
		}
	}
}

// --- Constraint-semantics tests -------------------------------------------
//
// The tests above only assert structural existence (tables/columns). These
// tests prove the two novel DB-level enforcement mechanisms (denormalized
// rarity + trigger + partial unique index / CHECK) actually reject the data
// they're meant to reject, not just that the schema exists.

// setupSchemaDB starts a fresh Postgres container, runs migrations up, and
// returns a connected *sql.DB. The container and DB are cleaned up via
// t.Cleanup.
func setupSchemaDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := startPostgres(t)

	if err := Up(databaseURL); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustInsertCharacter(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO characters (name, land_of_origin, stats) VALUES ($1, $2, $3) RETURNING id`,
		name, "Test Land", "",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert character %q: %v", name, err)
	}
	return id
}

func mustInsertGuild(t *testing.T, db *sql.DB, name string, leaderCharacterID int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO guilds (name, leader_character_id, land_of_origin) VALUES ($1, $2, $3) RETURNING id`,
		name, leaderCharacterID, "Test Land",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert guild %q: %v", name, err)
	}
	return id
}

func mustInsertItem(t *testing.T, db *sql.DB, name, rarity string, forgerCharacterID int64, price int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO items (name, land_of_origin, rarity, forger_character_id, price) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, "Test Land", rarity, forgerCharacterID, price,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert item %q: %v", name, err)
	}
	return id
}

// wantPQErrorCode fails the test unless err is a *pq.Error with the given
// SQLSTATE code (e.g. "23505" unique_violation, "23514" check_violation).
func wantPQErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a *pq.Error with code %s, got no error", code)
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		t.Fatalf("expected a *pq.Error, got %T: %v", err, err)
	}
	if string(pqErr.Code) != code {
		t.Errorf("expected pq error code %s, got %s (%v)", code, pqErr.Code, pqErr)
	}
}

func TestInventories_SecondGuildForLegendaryItem_Rejected(t *testing.T) {
	db := setupSchemaDB(t)

	leader1 := mustInsertCharacter(t, db, "Leader One")
	leader2 := mustInsertCharacter(t, db, "Leader Two")
	guild1 := mustInsertGuild(t, db, "Guild One", leader1)
	guild2 := mustInsertGuild(t, db, "Guild Two", leader2)
	item := mustInsertItem(t, db, "Soul Reaver", "LEGENDARY", leader1, 1000)

	if _, err := db.Exec(
		`INSERT INTO inventories (guild_id, item_id, quantity) VALUES ($1, $2, 1)`,
		guild1, item,
	); err != nil {
		t.Fatalf("first inventory insert for legendary item should succeed: %v", err)
	}

	_, err := db.Exec(
		`INSERT INTO inventories (guild_id, item_id, quantity) VALUES ($1, $2, 1)`,
		guild2, item,
	)
	// unique_violation from idx_inventories_legendary_unique (migration 000005).
	wantPQErrorCode(t, err, "23505")
}

func TestAuctions_NonLegendaryItem_Rejected(t *testing.T) {
	db := setupSchemaDB(t)

	leader := mustInsertCharacter(t, db, "Leader")
	guild := mustInsertGuild(t, db, "Guild", leader)
	commonItem := mustInsertItem(t, db, "Common Sword", "COMMON", leader, 50)

	now := time.Now().UTC()
	_, err := db.Exec(
		`INSERT INTO auctions (item_id, owner_guild_id, start_time, end_time, base_price)
		 VALUES ($1, $2, $3, $4, $5)`,
		commonItem, guild, now, now.Add(time.Hour), 50,
	)
	// check_violation from CHECK (item_rarity = 'LEGENDARY') (migration 000008).
	wantPQErrorCode(t, err, "23514")
}

func TestAuctions_SecondActiveAuctionForSameItem_Rejected(t *testing.T) {
	db := setupSchemaDB(t)

	leader := mustInsertCharacter(t, db, "Leader")
	guild := mustInsertGuild(t, db, "Guild", leader)
	legendaryItem := mustInsertItem(t, db, "Eye of the Dragon Ring", "LEGENDARY", leader, 5000)

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO auctions (item_id, owner_guild_id, start_time, end_time, base_price)
		 VALUES ($1, $2, $3, $4, $5)`,
		legendaryItem, guild, now, now.Add(time.Hour), 5000,
	); err != nil {
		t.Fatalf("first ACTIVE auction insert should succeed: %v", err)
	}

	_, err := db.Exec(
		`INSERT INTO auctions (item_id, owner_guild_id, start_time, end_time, base_price)
		 VALUES ($1, $2, $3, $4, $5)`,
		legendaryItem, guild, now, now.Add(2*time.Hour), 5000,
	)
	// unique_violation from idx_auctions_active_item_unique (migration 000008).
	wantPQErrorCode(t, err, "23505")
}
