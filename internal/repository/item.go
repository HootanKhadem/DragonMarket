package repository

import "context"

type ItemRarity string

const (
	RarityCommon    ItemRarity = "COMMON"
	RarityRare      ItemRarity = "RARE"
	RarityLegendary ItemRarity = "LEGENDARY"
)

// Item mirrors the items table.
type Item struct {
	ID                int64
	Name              string
	LandOfOrigin      string
	Rarity            ItemRarity
	ForgerCharacterID int64
	Price             int
}

type ItemRepository interface {
	Create(ctx context.Context, db DBTX, item Item) (Item, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Item, error)
	List(ctx context.Context, db DBTX, limit, offset int) ([]Item, error)
	ListByRarity(ctx context.Context, db DBTX, rarity ItemRarity) ([]Item, error)
	UpdatePrice(ctx context.Context, db DBTX, id int64, price int) error
}

type itemRepository struct{}

func NewItemRepository() ItemRepository {
	return &itemRepository{}
}

func (r *itemRepository) Create(ctx context.Context, db DBTX, item Item) (Item, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO items (name, land_of_origin, rarity, forger_character_id, price)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		item.Name, item.LandOfOrigin, item.Rarity, item.ForgerCharacterID, item.Price,
	).Scan(&item.ID)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r *itemRepository) GetByID(ctx context.Context, db DBTX, id int64) (Item, error) {
	var item Item
	err := db.QueryRow(ctx,
		`SELECT id, name, land_of_origin, rarity, forger_character_id, price
		 FROM items WHERE id = $1`,
		id,
	).Scan(&item.ID, &item.Name, &item.LandOfOrigin, &item.Rarity, &item.ForgerCharacterID, &item.Price)
	if err != nil {
		return Item{}, scanErr(err)
	}
	return item, nil
}

func (r *itemRepository) List(ctx context.Context, db DBTX, limit, offset int) ([]Item, error) {
	if limit <= 0 {
		limit = -1 // Postgres treats LIMIT -1 as "no limit"
	}
	rows, err := db.Query(ctx,
		`SELECT id, name, land_of_origin, rarity, forger_character_id, price
		 FROM items ORDER BY id LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (r *itemRepository) ListByRarity(ctx context.Context, db DBTX, rarity ItemRarity) ([]Item, error) {
	rows, err := db.Query(ctx,
		`SELECT id, name, land_of_origin, rarity, forger_character_id, price
		 FROM items WHERE rarity = $1 ORDER BY id`,
		rarity,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (r *itemRepository) UpdatePrice(ctx context.Context, db DBTX, id int64, price int) error {
	_, err := db.Exec(ctx, `UPDATE items SET price = $2 WHERE id = $1`, id, price)
	return err
}

func scanItems(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]Item, error) {
	var out []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Name, &item.LandOfOrigin, &item.Rarity, &item.ForgerCharacterID, &item.Price); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
