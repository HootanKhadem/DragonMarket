package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func init() {

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

var adminConnStr string

var dbNameCounter atomic.Uint64

var invalidDBNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = container.Terminate(context.Background())
		os.Exit(1)
	}
	adminConnStr = connStr

	code := m.Run()

	if err := container.Terminate(context.Background()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to terminate postgres container: %v\n", err)
	}

	os.Exit(code)
}

func sanitizeDBName(name string) string {
	cleaned := strings.ToLower(invalidDBNameChars.ReplaceAllString(name, "_"))

	const maxBase = 40
	if len(cleaned) > maxBase {
		cleaned = cleaned[:maxBase]
	}

	n := dbNameCounter.Add(1)
	return fmt.Sprintf("%s_%d", cleaned, n)
}

func freshDatabaseURL(t *testing.T) string {
	t.Helper()

	dbName := sanitizeDBName(t.Name())

	adminDB, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		cleanupDB, err := sql.Open("postgres", adminConnStr)
		if err != nil {
			t.Logf("cleanup: open admin connection for DROP DATABASE %s: %v", dbName, err)
			return
		}
		defer cleanupDB.Close()

		if _, err := cleanupDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, dbName)); err != nil {
			t.Logf("cleanup: drop database %s: %v", dbName, err)
		}
	})

	u, err := url.Parse(adminConnStr)
	if err != nil {
		t.Fatalf("parse admin connection string: %v", err)
	}
	u.Path = "/" + dbName
	return u.String()
}

func TestUp_CreatesExpectedTablesAndColumns(t *testing.T) {
	databaseURL := freshDatabaseURL(t)

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
	databaseURL := freshDatabaseURL(t)

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

func setupSchemaDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := freshDatabaseURL(t)

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
