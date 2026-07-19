package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type AuctionStatus string

const (
	AuctionActive  AuctionStatus = "ACTIVE"
	AuctionExpired AuctionStatus = "EXPIRED"
)

type Auction struct {
	ID           int64
	ItemID       int64
	OwnerGuildID int64
	Status       AuctionStatus
	StartTime    time.Time
	EndTime      time.Time
	BasePrice    int
	ItemRarity   ItemRarity // read-only, trigger-populated
}

type AuctionRepository interface {
	Create(ctx context.Context, db DBTX, a Auction) (Auction, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Auction, error)
	GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Auction, error)
	GetActiveByItemID(ctx context.Context, db DBTX, itemID int64) (Auction, error)
	ListActive(ctx context.Context, db DBTX) ([]Auction, error)
	ListActiveEndingBefore(ctx context.Context, db DBTX, cutoff time.Time) ([]Auction, error)
	Update(ctx context.Context, db DBTX, a Auction) error
}

type auctionRepository struct{}

func NewAuctionRepository() AuctionRepository {
	return &auctionRepository{}
}

const auctionColumns = `id, item_id, owner_guild_id, status, start_time, end_time, base_price, item_rarity`

func (r *auctionRepository) Create(ctx context.Context, db DBTX, auction Auction) (Auction, error) {
	if auction.Status == "" {
		auction.Status = AuctionActive
	}
	err := db.QueryRow(ctx,
		`INSERT INTO auctions (item_id, owner_guild_id, status, start_time, end_time, base_price)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, item_rarity`,
		auction.ItemID, auction.OwnerGuildID, auction.Status, auction.StartTime, auction.EndTime, auction.BasePrice,
	).Scan(&auction.ID, &auction.ItemRarity)
	if err != nil {
		return Auction{}, err
	}
	return auction, nil
}

func (r *auctionRepository) GetByID(ctx context.Context, db DBTX, id int64) (Auction, error) {
	return r.scanOne(db.QueryRow(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE id = $1`, id))
}

func (r *auctionRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Auction, error) {
	return r.scanOne(tx.QueryRow(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE id = $1 FOR UPDATE`, id))
}

func (r *auctionRepository) GetActiveByItemID(ctx context.Context, db DBTX, itemID int64) (Auction, error) {
	return r.scanOne(db.QueryRow(ctx,
		`SELECT `+auctionColumns+` FROM auctions WHERE item_id = $1 AND status = 'ACTIVE'`,
		itemID,
	))
}

func (r *auctionRepository) scanOne(row pgx.Row) (Auction, error) {
	var auction Auction
	err := row.Scan(&auction.ID, &auction.ItemID, &auction.OwnerGuildID, &auction.Status, &auction.StartTime, &auction.EndTime,
		&auction.BasePrice, &auction.ItemRarity)
	if err != nil {
		return Auction{}, scanErr(err)
	}
	return auction, nil
}

func (r *auctionRepository) ListActive(ctx context.Context, db DBTX) ([]Auction, error) {
	return r.scanMany(db.Query(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE status = 'ACTIVE' ORDER BY id`))
}

func (r *auctionRepository) ListActiveEndingBefore(ctx context.Context, db DBTX, cutoff time.Time) ([]Auction, error) {
	return r.scanMany(db.Query(ctx,
		`SELECT `+auctionColumns+` FROM auctions WHERE status = 'ACTIVE' AND end_time <= $1 ORDER BY id`,
		cutoff,
	))
}

func (r *auctionRepository) scanMany(rows pgx.Rows, err error) ([]Auction, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var auctions []Auction
	for rows.Next() {
		var a Auction
		if err := rows.Scan(&a.ID, &a.ItemID, &a.OwnerGuildID, &a.Status, &a.StartTime, &a.EndTime, &a.BasePrice, &a.ItemRarity); err != nil {
			return nil, err
		}
		auctions = append(auctions, a)
	}
	return auctions, rows.Err()
}

func (r *auctionRepository) Update(ctx context.Context, db DBTX, auction Auction) error {
	_, err := db.Exec(ctx,
		`UPDATE auctions SET status = $2, end_time = $3 WHERE id = $1`,
		auction.ID, auction.Status, auction.EndTime,
	)
	return err
}
