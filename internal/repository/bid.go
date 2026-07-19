package repository

import (
	"context"
	"time"
)

// BidStatus mirrors the bid_status Postgres enum.
type BidStatus string

const (
	BidActive    BidStatus = "ACTIVE"
	BidCancelled BidStatus = "CANCELLED"
	BidWon       BidStatus = "WON"
	BidLost      BidStatus = "LOST"
)

// Bid mirrors the bids table.
type Bid struct {
	ID        int64
	AuctionID int64
	GuildID   int64
	Amount    int
	Status    BidStatus
	CreatedAt time.Time
}

type BidRepository interface {
	Create(ctx context.Context, db DBTX, b Bid) (Bid, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Bid, error)
	ListByAuctionID(ctx context.Context, db DBTX, auctionID int64) ([]Bid, error)
	GetHighestActiveByAuctionID(ctx context.Context, db DBTX, auctionID int64) (Bid, error)
	Update(ctx context.Context, db DBTX, b Bid) error
}

type bidRepository struct{}

func NewBidRepository() BidRepository {
	return &bidRepository{}
}

const bidColumns = `id, auction_id, guild_id, amount, status, created_at`

func (r *bidRepository) Create(ctx context.Context, db DBTX, b Bid) (Bid, error) {
	if b.Status == "" {
		b.Status = BidActive
	}
	err := db.QueryRow(ctx,
		`INSERT INTO bids (auction_id, guild_id, amount, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		b.AuctionID, b.GuildID, b.Amount, b.Status,
	).Scan(&b.ID, &b.CreatedAt)
	if err != nil {
		return Bid{}, err
	}
	return b, nil
}

func (r *bidRepository) GetByID(ctx context.Context, db DBTX, id int64) (Bid, error) {
	var b Bid
	err := db.QueryRow(ctx, `SELECT `+bidColumns+` FROM bids WHERE id = $1`, id).
		Scan(&b.ID, &b.AuctionID, &b.GuildID, &b.Amount, &b.Status, &b.CreatedAt)
	if err != nil {
		return Bid{}, scanErr(err)
	}
	return b, nil
}

func (r *bidRepository) ListByAuctionID(ctx context.Context, db DBTX, auctionID int64) ([]Bid, error) {
	rows, err := db.Query(ctx, `SELECT `+bidColumns+` FROM bids WHERE auction_id = $1 ORDER BY id`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Bid
	for rows.Next() {
		var b Bid
		if err := rows.Scan(&b.ID, &b.AuctionID, &b.GuildID, &b.Amount, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *bidRepository) GetHighestActiveByAuctionID(ctx context.Context, db DBTX, auctionID int64) (Bid, error) {
	var b Bid
	err := db.QueryRow(ctx,
		`SELECT `+bidColumns+` FROM bids
		 WHERE auction_id = $1 AND status = 'ACTIVE'
		 ORDER BY amount DESC, created_at ASC
		 LIMIT 1`,
		auctionID,
	).Scan(&b.ID, &b.AuctionID, &b.GuildID, &b.Amount, &b.Status, &b.CreatedAt)
	if err != nil {
		return Bid{}, scanErr(err)
	}
	return b, nil
}

func (r *bidRepository) Update(ctx context.Context, db DBTX, b Bid) error {
	_, err := db.Exec(ctx, `UPDATE bids SET status = $2 WHERE id = $1`, b.ID, b.Status)
	return err
}
