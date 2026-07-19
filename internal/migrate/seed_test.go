package migrate

import (
	"database/sql"
	"testing"
)

// TestSeed_* verify the shape of the data inserted by migration
// 000011_seed_data (Task 4), against a real Postgres via testcontainers-go.
// setupSchemaDB (defined in migrate_test.go) already runs Up() -- which
// includes the seed migration -- against a fresh container.

func scalarInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func TestSeed_Guilds(t *testing.T) {
	db := setupSchemaDB(t)

	guildCount := scalarInt(t, db, `SELECT COUNT(*) FROM guilds`)
	if guildCount != 5 {
		t.Errorf("expected exactly 5 guilds, got %d", guildCount)
	}

	vorynthaxGuildCount := scalarInt(t, db, `SELECT COUNT(*) FROM guilds WHERE name = 'Vorynthax Guild'`)
	if vorynthaxGuildCount != 1 {
		t.Errorf("expected exactly 1 guild named 'Vorynthax Guild', got %d", vorynthaxGuildCount)
	}
}

func TestSeed_Characters(t *testing.T) {
	db := setupSchemaDB(t)

	characterCount := scalarInt(t, db, `SELECT COUNT(*) FROM characters`)
	if characterCount != 20 {
		t.Errorf("expected exactly 20 characters total, got %d", characterCount)
	}

	guildedCharacterCount := scalarInt(t, db, `SELECT COUNT(*) FROM characters WHERE guild_id IS NOT NULL`)
	if guildedCharacterCount != 10 {
		t.Errorf("expected exactly 10 characters with a guild, got %d", guildedCharacterCount)
	}

	guildlessCharacterCount := scalarInt(t, db, `SELECT COUNT(*) FROM characters WHERE guild_id IS NULL`)
	if guildlessCharacterCount != 10 {
		t.Errorf("expected exactly 10 characters with guild_id NULL (blacksmiths), got %d", guildlessCharacterCount)
	}
}

func TestSeed_Items(t *testing.T) {
	db := setupSchemaDB(t)

	itemCount := scalarInt(t, db, `SELECT COUNT(*) FROM items`)
	if itemCount != 30 {
		t.Errorf("expected exactly 30 items total, got %d", itemCount)
	}

	cases := map[string]int{
		"COMMON":    20,
		"RARE":      7,
		"LEGENDARY": 3,
	}
	for rarity, want := range cases {
		rarityItemCount := scalarInt(t, db, `SELECT COUNT(*) FROM items WHERE rarity = $1`, rarity)
		if rarityItemCount != want {
			t.Errorf("expected %d %s items, got %d", want, rarity, rarityItemCount)
		}
	}
}

func TestSeed_LegendaryOwnership(t *testing.T) {
	db := setupSchemaDB(t)

	// Soul Reaver and Eye of the Dragon Ring must both be owned (via an
	// inventories row) by Vorynthax Guild specifically.
	for _, name := range []string{"Soul Reaver", "Eye of the Dragon Ring"} {
		ownershipCount := scalarInt(t, db, `
			SELECT COUNT(*)
			FROM inventories inv
			JOIN items i ON i.id = inv.item_id
			JOIN guilds g ON g.id = inv.guild_id
			WHERE i.name = $1 AND g.name = 'Vorynthax Guild' AND inv.quantity = 1
		`, name)
		if ownershipCount != 1 {
			t.Errorf("expected %q to have exactly one inventory row owned by Vorynthax Guild with quantity 1, got %d matching rows", name, ownershipCount)
		}
	}

	// The third legendary item must exist, have an inventory row, and NOT be
	// owned by Vorynthax Guild.
	var thirdName string
	err := db.QueryRow(`
		SELECT name FROM items
		WHERE rarity = 'LEGENDARY' AND name NOT IN ('Soul Reaver', 'Eye of the Dragon Ring')
	`).Scan(&thirdName)
	if err != nil {
		t.Fatalf("expected a third legendary item, query error: %v", err)
	}

	thirdItemOwnershipCount := scalarInt(t, db, `
		SELECT COUNT(*)
		FROM inventories inv
		JOIN items i ON i.id = inv.item_id
		JOIN guilds g ON g.id = inv.guild_id
		WHERE i.name = $1 AND g.name <> 'Vorynthax Guild' AND inv.quantity = 1
	`, thirdName)
	if thirdItemOwnershipCount != 1 {
		t.Errorf("expected third legendary item %q to have exactly one inventory row owned by a non-Vorynthax guild, got %d matching rows", thirdName, thirdItemOwnershipCount)
	}

	// Every LEGENDARY item has exactly one inventories row system-wide.
	legendaryInventoryCount := scalarInt(t, db, `
		SELECT COUNT(*)
		FROM inventories inv
		JOIN items i ON i.id = inv.item_id
		WHERE i.rarity = 'LEGENDARY'
	`)
	if legendaryInventoryCount != 3 {
		t.Errorf("expected exactly 3 inventories rows for LEGENDARY items, got %d", legendaryInventoryCount)
	}
}

func TestSeed_ListingsAndOutOfScopeTables(t *testing.T) {
	db := setupSchemaDB(t)

	activeListingCount := scalarInt(t, db, `SELECT COUNT(*) FROM listings WHERE status = 'ACTIVE'`)
	if activeListingCount != 27 {
		t.Errorf("expected exactly 27 ACTIVE listings, got %d", activeListingCount)
	}

	totalListingCount := scalarInt(t, db, `SELECT COUNT(*) FROM listings`)
	if totalListingCount != 27 {
		t.Errorf("expected exactly 27 listings total (no non-ACTIVE ones seeded), got %d", totalListingCount)
	}

	// Out of scope for this seed: no auctions, bids, or transaction_logs.
	for _, table := range []string{"auctions", "bids", "transaction_logs"} {
		emptyTableCount := scalarInt(t, db, `SELECT COUNT(*) FROM `+table)
		if emptyTableCount != 0 {
			t.Errorf("expected 0 rows in %s, got %d", table, emptyTableCount)
		}
	}
}

func TestSeed_GoldPouches(t *testing.T) {
	db := setupSchemaDB(t)

	goldPouchCount := scalarInt(t, db, `SELECT COUNT(*) FROM gold_pouches`)
	if goldPouchCount != 5 {
		t.Errorf("expected exactly 5 gold_pouches (one per guild), got %d", goldPouchCount)
	}

	invalidSpendingLimitCount := scalarInt(t, db, `
		SELECT COUNT(*) FROM gold_pouches
		WHERE daily_spending_limit < 2000 OR daily_spending_limit > 10000 OR daily_spending_limit % 100 <> 0
	`)
	if invalidSpendingLimitCount != 0 {
		t.Errorf("expected every gold_pouch.daily_spending_limit to be in [2000,10000] and a multiple of 100, %d rows violate this", invalidSpendingLimitCount)
	}
}

// TestSeed_Idempotent verifies that running Up() again against an
// already-seeded database (as would happen on a normal app restart) does not
// duplicate rows or error -- golang-migrate's schema_migrations tracking
// should make the second Up() call a pure no-op.
func TestSeed_Idempotent(t *testing.T) {
	databaseURL := freshDatabaseURL(t)

	if err := Up(databaseURL); err != nil {
		t.Fatalf("first Up() error = %v", err)
	}
	if err := Up(databaseURL); err != nil {
		t.Fatalf("second Up() error = %v", err)
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	guildCount := scalarInt(t, db, `SELECT COUNT(*) FROM guilds`)
	if guildCount != 5 {
		t.Errorf("expected exactly 5 guilds after two Up() calls, got %d (rows duplicated)", guildCount)
	}
	itemCount := scalarInt(t, db, `SELECT COUNT(*) FROM items`)
	if itemCount != 30 {
		t.Errorf("expected exactly 30 items after two Up() calls, got %d (rows duplicated)", itemCount)
	}
}
