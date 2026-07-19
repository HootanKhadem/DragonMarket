package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Inventory mirrors the inventories table. ItemRarity is a denormalized,
// trigger-populated copy of the owning item's rarity (see migration
// 000005) — repository methods here NEVER set it in an INSERT/UPDATE
// statement; it is always read back from what the DB trigger computed.
type Inventory struct {
	ID         int64
	GuildID    int64
	ItemID     int64
	Quantity   int
	ItemRarity ItemRarity // read-only, trigger-populated
}

type InventoryRepository interface {
	Create(ctx context.Context, db DBTX, inv Inventory) (Inventory, error)
	GetByGuildAndItem(ctx context.Context, db DBTX, guildID, itemID int64) (Inventory, error)
	GetByGuildAndItemForUpdate(ctx context.Context, tx pgx.Tx, guildID, itemID int64) (Inventory, error)
	ListByGuildID(ctx context.Context, db DBTX, guildID int64) ([]Inventory, error)
	UpdateQuantity(ctx context.Context, db DBTX, guildID, itemID int64, quantity int) error
	Upsert(ctx context.Context, db DBTX, guildID, itemID int64, quantity int) (Inventory, error)
	Delete(ctx context.Context, db DBTX, id int64) error
}

type inventoryRepository struct{}

// NewInventoryRepository returns a pgx-backed InventoryRepository.
func NewInventoryRepository() InventoryRepository {
	return &inventoryRepository{}
}

func (r *inventoryRepository) Create(ctx context.Context, db DBTX, inv Inventory) (Inventory, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO inventories (guild_id, item_id, quantity)
		 VALUES ($1, $2, $3)
		 RETURNING id, item_rarity`,
		inv.GuildID, inv.ItemID, inv.Quantity,
	).Scan(&inv.ID, &inv.ItemRarity)
	if err != nil {
		return Inventory{}, err
	}
	return inv, nil
}

func (r *inventoryRepository) GetByGuildAndItem(ctx context.Context, db DBTX, guildID, itemID int64) (Inventory, error) {
	return r.getBy(ctx, db, guildID, itemID, false)
}

func (r *inventoryRepository) GetByGuildAndItemForUpdate(ctx context.Context, tx pgx.Tx, guildID, itemID int64) (Inventory, error) {
	return r.getBy(ctx, tx, guildID, itemID, true)
}

func (r *inventoryRepository) getBy(ctx context.Context, db DBTX, guildID, itemID int64, forUpdate bool) (Inventory, error) {
	query := `SELECT id, guild_id, item_id, quantity, item_rarity
	          FROM inventories WHERE guild_id = $1 AND item_id = $2`
	if forUpdate {
		query += " FOR UPDATE"
	}
	var inv Inventory
	err := db.QueryRow(ctx, query, guildID, itemID).
		Scan(&inv.ID, &inv.GuildID, &inv.ItemID, &inv.Quantity, &inv.ItemRarity)
	if err != nil {
		return Inventory{}, scanErr(err)
	}
	return inv, nil
}

func (r *inventoryRepository) ListByGuildID(ctx context.Context, db DBTX, guildID int64) ([]Inventory, error) {
	rows, err := db.Query(ctx,
		`SELECT id, guild_id, item_id, quantity, item_rarity
		 FROM inventories WHERE guild_id = $1 ORDER BY id`,
		guildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Inventory
	for rows.Next() {
		var inv Inventory
		if err := rows.Scan(&inv.ID, &inv.GuildID, &inv.ItemID, &inv.Quantity, &inv.ItemRarity); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *inventoryRepository) UpdateQuantity(ctx context.Context, db DBTX, guildID, itemID int64, quantity int) error {
	_, err := db.Exec(ctx,
		`UPDATE inventories SET quantity = $3 WHERE guild_id = $1 AND item_id = $2`,
		guildID, itemID, quantity,
	)
	return err
}

func (r *inventoryRepository) Upsert(ctx context.Context, db DBTX, guildID, itemID int64, quantity int) (Inventory, error) {
	var inv Inventory
	err := db.QueryRow(ctx,
		`INSERT INTO inventories (guild_id, item_id, quantity)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (guild_id, item_id) DO UPDATE SET quantity = EXCLUDED.quantity
		 RETURNING id, guild_id, item_id, quantity, item_rarity`,
		guildID, itemID, quantity,
	).Scan(&inv.ID, &inv.GuildID, &inv.ItemID, &inv.Quantity, &inv.ItemRarity)
	if err != nil {
		return Inventory{}, err
	}
	return inv, nil
}

func (r *inventoryRepository) Delete(ctx context.Context, db DBTX, id int64) error {
	_, err := db.Exec(ctx, `DELETE FROM inventories WHERE id = $1`, id)
	return err
}
