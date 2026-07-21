package settlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

var ErrAuctionNotEligible = errors.New("settlement: auction is no longer eligible for settlement")

type Settler struct {
	db          service.TxPool
	auctions    repository.AuctionRepository
	bids        repository.BidRepository
	inventories repository.InventoryRepository
	goldPouches *service.GoldPouchService
	interval    time.Duration

	Logger *slog.Logger

	OnTick func(err error)
}

func NewSettler(
	db service.TxPool,
	auctions repository.AuctionRepository,
	bids repository.BidRepository,
	inventories repository.InventoryRepository,
	goldPouches *service.GoldPouchService,
	interval time.Duration,
) *Settler {
	return &Settler{
		db:          db,
		auctions:    auctions,
		bids:        bids,
		inventories: inventories,
		goldPouches: goldPouches,
		interval:    interval,
	}
}

func (s *Settler) Tick(ctx context.Context) error {
	cutoff := time.Now().UTC()
	auctions, err := s.auctions.ListActiveEndingBefore(ctx, s.db, cutoff)
	if err != nil {
		return fmt.Errorf("settlement: list auctions ending before %v: %w", cutoff, err)
	}

	for _, a := range auctions {
		if err := s.SettleAuction(ctx, a.ID); err != nil {
			if errors.Is(err, ErrAuctionNotEligible) {
				continue
			}
			s.logger().Warn("failed to settle auction, will retry next tick", "auction_id", a.ID, "error", err)
		}
	}
	return nil
}

func (s *Settler) SettleAuction(ctx context.Context, auctionID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("settlement: settle auction %d: begin tx: %w", auctionID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	auction, err := s.auctions.GetByIDForUpdate(ctx, tx, auctionID)
	if err != nil {
		return fmt.Errorf("settlement: settle auction %d: lock: %w", auctionID, err)
	}

	now := time.Now().UTC()
	if auction.Status != repository.AuctionActive || !now.After(auction.EndTime) {
		return ErrAuctionNotEligible
	}

	reference := fmt.Sprintf("auction:%d", auction.ID)

	winner, err := s.bids.GetHighestActiveByAuctionID(ctx, tx, auction.ID)
	switch {
	case err == nil:
		if err := s.settleWithWinner(ctx, tx, auction, winner, reference); err != nil {
			return err
		}
	case errors.Is(err, repository.ErrNotFound):
		// No bids at all: item remains with its current owner, available
		// for a future auction per the Global Constraints.
	default:
		return fmt.Errorf("settlement: settle auction %d: get highest bid: %w", auctionID, err)
	}

	auction.Status = repository.AuctionExpired
	if err := s.auctions.Update(ctx, tx, auction); err != nil {
		return fmt.Errorf("settlement: settle auction %d: expire: %w", auctionID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("settlement: settle auction %d: commit: %w", auctionID, err)
	}
	committed = true
	return nil
}

func (s *Settler) settleWithWinner(ctx context.Context, tx pgx.Tx, auction repository.Auction, winner repository.Bid, reference string) error {
	if err := s.goldPouches.Settle(ctx, tx, winner.GuildID, winner.Amount, repository.TxAuctionWin, &reference); err != nil {
		return fmt.Errorf("settlement: settle auction %d: settle winner reservation: %w", auction.ID, err)
	}
	if err := s.goldPouches.Credit(ctx, tx, auction.OwnerGuildID, winner.Amount, &reference); err != nil {
		return fmt.Errorf("settlement: settle auction %d: credit seller: %w", auction.ID, err)
	}

	sellerInv, err := s.inventories.GetByGuildAndItemForUpdate(ctx, tx, auction.OwnerGuildID, auction.ItemID)
	if err != nil {
		return fmt.Errorf("settlement: settle auction %d: lock seller inventory: %w", auction.ID, err)
	}
	if err := s.inventories.Delete(ctx, tx, sellerInv.ID); err != nil {
		return fmt.Errorf("settlement: settle auction %d: delete seller inventory: %w", auction.ID, err)
	}
	if _, err := s.inventories.Upsert(ctx, tx, winner.GuildID, auction.ItemID, 1); err != nil {
		return fmt.Errorf("settlement: settle auction %d: upsert winner inventory: %w", auction.ID, err)
	}

	winner.Status = repository.BidWon
	if err := s.bids.Update(ctx, tx, winner); err != nil {
		return fmt.Errorf("settlement: settle auction %d: mark winning bid won: %w", auction.ID, err)
	}

	allBids, err := s.bids.ListByAuctionID(ctx, tx, auction.ID)
	if err != nil {
		return fmt.Errorf("settlement: settle auction %d: list bids: %w", auction.ID, err)
	}
	for _, b := range allBids {
		if b.ID == winner.ID || b.Status != repository.BidActive {
			continue
		}
		if err := s.goldPouches.Release(ctx, tx, b.GuildID, b.Amount, &reference); err != nil {
			return fmt.Errorf("settlement: settle auction %d: release bid %d: %w", auction.ID, b.ID, err)
		}
		b.Status = repository.BidLost
		if err := s.bids.Update(ctx, tx, b); err != nil {
			return fmt.Errorf("settlement: settle auction %d: mark bid %d lost: %w", auction.ID, b.ID, err)
		}
	}
	return nil
}

func (s *Settler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.Tick(ctx)
			if err != nil {
				s.logger().Warn("settlement sweep tick failed", "error", err)
			}
			if s.OnTick != nil {
				s.OnTick(err)
			}
		}
	}
}

func (s *Settler) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
