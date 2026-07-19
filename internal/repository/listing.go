package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ListingStatus string

const (
	ListingActive  ListingStatus = "ACTIVE"
	ListingExpired ListingStatus = "EXPIRED"
)

// Listing mirrors the listings table.
type Listing struct {
	ID        int64
	ItemID    int64
	GuildID   int64
	Quantity  int
	BasePrice int
	Status    ListingStatus
}

type ListingRepository interface {
	Create(ctx context.Context, db DBTX, l Listing) (Listing, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Listing, error)
	GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Listing, error)
	GetActiveByItemID(ctx context.Context, db DBTX, itemID int64) (Listing, error)
	ListByStatus(ctx context.Context, db DBTX, status ListingStatus) ([]Listing, error)
	Update(ctx context.Context, db DBTX, l Listing) error
}

type listingRepository struct{}

func NewListingRepository() ListingRepository {
	return &listingRepository{}
}

const listingColumns = `id, item_id, guild_id, quantity, base_price, status`

func (r *listingRepository) Create(ctx context.Context, db DBTX, l Listing) (Listing, error) {
	if l.Status == "" {
		l.Status = ListingActive
	}
	err := db.QueryRow(ctx,
		`INSERT INTO listings (item_id, guild_id, quantity, base_price, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		l.ItemID, l.GuildID, l.Quantity, l.BasePrice, l.Status,
	).Scan(&l.ID)
	if err != nil {
		return Listing{}, err
	}
	return l, nil
}

func (r *listingRepository) GetByID(ctx context.Context, db DBTX, id int64) (Listing, error) {
	return r.scanOne(db.QueryRow(ctx, `SELECT `+listingColumns+` FROM listings WHERE id = $1`, id))
}

func (r *listingRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Listing, error) {
	return r.scanOne(tx.QueryRow(ctx, `SELECT `+listingColumns+` FROM listings WHERE id = $1 FOR UPDATE`, id))
}

func (r *listingRepository) GetActiveByItemID(ctx context.Context, db DBTX, itemID int64) (Listing, error) {
	return r.scanOne(db.QueryRow(ctx,
		`SELECT `+listingColumns+` FROM listings WHERE item_id = $1 AND status = 'ACTIVE'`,
		itemID,
	))
}

func (r *listingRepository) scanOne(row pgx.Row) (Listing, error) {
	var l Listing
	err := row.Scan(&l.ID, &l.ItemID, &l.GuildID, &l.Quantity, &l.BasePrice, &l.Status)
	if err != nil {
		return Listing{}, scanErr(err)
	}
	return l, nil
}

func (r *listingRepository) ListByStatus(ctx context.Context, db DBTX, status ListingStatus) ([]Listing, error) {
	rows, err := db.Query(ctx, `SELECT `+listingColumns+` FROM listings WHERE status = $1 ORDER BY id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Listing
	for rows.Next() {
		var l Listing
		if err := rows.Scan(&l.ID, &l.ItemID, &l.GuildID, &l.Quantity, &l.BasePrice, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *listingRepository) Update(ctx context.Context, db DBTX, l Listing) error {
	_, err := db.Exec(ctx,
		`UPDATE listings SET quantity = $2, base_price = $3, status = $4 WHERE id = $1`,
		l.ID, l.Quantity, l.BasePrice, l.Status,
	)
	return err
}
