package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

var (
	ErrInvalidDuration      = errors.New("service: duration_seconds must be positive")
	ErrNotItemOwner         = errors.New("service: caller does not own this item")
	ErrItemNotLegendary     = errors.New("service: only LEGENDARY items can be auctioned")
	ErrAuctionAlreadyExists = errors.New("service: item already has an active auction")
	ErrAuctionNotFound      = errors.New("service: auction not found")
	ErrAuctionNotActive     = errors.New("service: auction is not active")
	ErrAuctionExpired       = errors.New("service: auction has passed its end time")
	ErrSelfBidNotAllowed    = errors.New("service: cannot bid on an auction for an item you own")
	ErrBidTooLow            = errors.New("service: bid does not meet the minimum required amount")
	ErrBidNotFound          = errors.New("service: bid not found")
	ErrNotBidOwner          = errors.New("service: caller does not own this bid")
	ErrBidNotActive         = errors.New("service: bid is not active")
	ErrBidIsHighest         = errors.New("service: cannot cancel the current highest bid")
)

// bidExtensionWindow/bidExtensionAmount implement the Global Constraints
// "sniping protection" rule: a qualifying bid arriving within this window of
// the CURRENT end_time pushes end_time out by this same amount, repeatable
// on every subsequent qualifying bid (see PlaceBid).
const (
	bidExtensionWindow = 5 * time.Minute
	bidExtensionAmount = 5 * time.Minute

	// minBidNumerator/minBidDenominator encode the 105%-of-highest floor as
	// an integer ratio so PlaceBid never touches floating point for a
	// money computation (see ceilDiv's doc comment for the rounding
	// rationale).
	minBidNumerator   = 105
	minBidDenominator = 100
)

// AuctionService owns auction creation/lookup/listing and bid
// placement/cancellation. It composes ItemRepository, InventoryRepository,
// AuctionRepository, BidRepository, and GoldPouchService, so -- like
// ItemService (Task 8) and unlike GoldPouchService (Task 7) -- it manages
// its own transaction boundaries rather than requiring the caller to pass
// one in.
type AuctionService struct {
	db          TxPool
	auctions    repository.AuctionRepository
	bids        repository.BidRepository
	items       repository.ItemRepository
	inventories repository.InventoryRepository
	goldPouches *GoldPouchService
	priceCache  *oracle.Cache
}

func NewAuctionService(
	db TxPool,
	auctions repository.AuctionRepository,
	bids repository.BidRepository,
	items repository.ItemRepository,
	inventories repository.InventoryRepository,
	goldPouches *GoldPouchService,
	priceCache *oracle.Cache,
) *AuctionService {
	return &AuctionService{
		db:          db,
		auctions:    auctions,
		bids:        bids,
		items:       items,
		inventories: inventories,
		goldPouches: goldPouches,
		priceCache:  priceCache,
	}
}

type CreateAuctionInput struct {
	ItemID int64
	// OwnerGuildID is the caller, taken from X-Guild-ID at the handler
	// layer -- it must match the item's actual owner (checked via
	// InventoryRepository.GetByGuildAndItem), never trusted at face value.
	OwnerGuildID int64
	// DurationSeconds is how long the auction runs from creation
	// (start_time = now, end_time = now + DurationSeconds). A plain
	// integer-seconds field was chosen over e.g. an RFC3339 end_time
	// string so the caller can't set a start_time in the past/future or an
	// end_time inconsistent with "starts now", and so validation
	// (>0) is a single int comparison.
	DurationSeconds int
}

type AuctionView struct {
	ID           int64
	ItemID       int64
	OwnerGuildID int64
	Status       repository.AuctionStatus
	StartTime    time.Time
	EndTime      time.Time
	BasePrice    int
}

type PlaceBidInput struct {
	ItemID  int64
	GuildID int64
	Amount  int
}

type BidView struct {
	ID        int64
	AuctionID int64
	GuildID   int64
	Amount    int
	Status    repository.BidStatus
	CreatedAt time.Time
}

type CancelBidInput struct {
	ItemID int64
	BidID  int64
	// GuildID is the caller (X-Guild-ID). The brief only mandates rejecting
	// a cancel when the auction isn't ACTIVE or when the bid is the current
	// highest, but allowing any guild to cancel any other guild's ACTIVE
	// bid would let one guild grief another's standing bids out of an
	// auction for free, so CancelBid additionally requires GuildID to match
	// the bid's own GuildID (ErrNotBidOwner otherwise). This is a
	// deliberate addition beyond the letter of the brief; documented here
	// and in the task report.
	GuildID int64
}

// CreateAuction validates that OwnerGuildID actually owns ItemID (via
// inventory), that the item is LEGENDARY, and that it doesn't already have
// an ACTIVE auction (the latter enforced by the DB's partial unique index
// on auctions(item_id) WHERE status='ACTIVE' -- a unique-violation from
// AuctionRepository.Create is translated to ErrAuctionAlreadyExists rather
// than leaking a raw pg error).
func (s *AuctionService) CreateAuction(ctx context.Context, in CreateAuctionInput) (AuctionView, error) {
	if in.DurationSeconds <= 0 {
		return AuctionView{}, ErrInvalidDuration
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AuctionView{}, fmt.Errorf("service: create auction: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	item, err := s.items.GetByID(ctx, tx, in.ItemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return AuctionView{}, ErrItemNotFound
		}
		return AuctionView{}, fmt.Errorf("service: create auction: get item: %w", err)
	}

	if _, err := s.inventories.GetByGuildAndItem(ctx, tx, in.OwnerGuildID, in.ItemID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return AuctionView{}, ErrNotItemOwner
		}
		return AuctionView{}, fmt.Errorf("service: create auction: check ownership: %w", err)
	}

	if item.Rarity != repository.RarityLegendary {
		return AuctionView{}, ErrItemNotLegendary
	}

	// base_price is resolved cache-first with a DB fallback, the same way
	// ItemService.toView resolves the price shown by GET /items/{id}: this
	// keeps "the current price of a legendary item" consistent between the
	// two endpoints, rather than an auction silently starting from a
	// stale DB price the oracle has already moved past.
	basePrice := s.currentPrice(item)

	now := time.Now().UTC().Truncate(time.Microsecond)
	auction, err := s.auctions.Create(ctx, tx, repository.Auction{
		ItemID:       in.ItemID,
		OwnerGuildID: in.OwnerGuildID,
		StartTime:    now,
		EndTime:      now.Add(time.Duration(in.DurationSeconds) * time.Second),
		BasePrice:    basePrice,
	})
	if err != nil {
		if isConflictErr(err) {
			return AuctionView{}, ErrAuctionAlreadyExists
		}
		return AuctionView{}, fmt.Errorf("service: create auction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return AuctionView{}, fmt.Errorf("service: create auction: commit: %w", err)
	}
	committed = true

	return toAuctionView(auction), nil
}

// GetAuction returns ErrAuctionNotFound both when no auction with this ID
// exists and when it exists but has already expired: per the Global
// Constraints, an EXPIRED auction must never be handed back as if it were
// still live. This mirrors ItemService.GetItem's not-found handling (the
// simplest of the two brief-sanctioned options) rather than inventing a
// separate "expired" response shape.
func (s *AuctionService) GetAuction(ctx context.Context, id int64) (AuctionView, error) {
	a, err := s.auctions.GetByID(ctx, s.db, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return AuctionView{}, ErrAuctionNotFound
		}
		return AuctionView{}, fmt.Errorf("service: get auction: %w", err)
	}
	if a.Status != repository.AuctionActive {
		return AuctionView{}, ErrAuctionNotFound
	}
	return toAuctionView(a), nil
}

// ListActiveAuctions lists ACTIVE auctions with limit/offset pagination.
// AuctionRepository.ListActive (Task 5, ground truth) has no limit/offset
// parameters, so pagination is applied here in the service layer over the
// full result slice rather than modifying that repository file. This is a
// known scalability gap (every call fetches every ACTIVE auction row) that
// would need revisiting if the active-auction count grows large; acceptable
// for this task per the brief's allowance to implement pagination "somehow"
// without touching the Task 5 file.
func (s *AuctionService) ListActiveAuctions(ctx context.Context, limit, offset int) ([]AuctionView, error) {
	all, err := s.auctions.ListActive(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("service: list active auctions: %w", err)
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(all) {
		return []AuctionView{}, nil
	}
	end := offset + limit
	if end > len(all) || end < offset {
		end = len(all)
	}
	slice := all[offset:end]
	views := make([]AuctionView, len(slice))
	for i, a := range slice {
		views[i] = toAuctionView(a)
	}
	return views, nil
}

// PlaceBid resolves the item's current ACTIVE auction, rejects self-bids and
// bids after end_time (even if the row hasn't been swept to EXPIRED yet by
// Task 10's background job), enforces the bid floor, and -- inside a single
// locked transaction -- reserves the bidder's funds and inserts the bid,
// extending end_time if the bid landed inside the sniping window.
//
// Locking design (this is the one subtlety worth over-documenting): after
// confirming an active auction exists and rejecting a self-bid using the
// unlocked row (self-bid only needs OwnerGuildID, which never changes after
// creation), PlaceBid re-reads the auction with GetByIDForUpdate. Every
// check that depends on data which can change concurrently -- auction
// status, end_time (extended by a previous concurrent bid), and the current
// highest ACTIVE bid used to compute the 105% floor -- is evaluated only
// after this lock is held, and the funds-reserve + bid-insert + (maybe)
// end_time-extend all happen before it's released. Any other PlaceBid or
// CancelBid call for the SAME auction must acquire this same row lock
// before it can read-or-write anything auction/bid-scoped, so concurrent
// attempts serialize into a single queue ordered by lock acquisition --
// which is exactly what the mandated concurrency test verifies (see
// TestAuctionService_PlaceBid_ConcurrentBidsAtSameFloor_OnlyOneAccepted).
func (s *AuctionService) PlaceBid(ctx context.Context, in PlaceBidInput) (BidView, error) {
	if in.Amount <= 0 {
		return BidView{}, ErrInvalidAmount
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BidView{}, fmt.Errorf("service: place bid: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	activeAuction, err := s.auctions.GetActiveByItemID(ctx, tx, in.ItemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return BidView{}, ErrAuctionNotFound
		}
		return BidView{}, fmt.Errorf("service: place bid: get active auction: %w", err)
	}

	if activeAuction.OwnerGuildID == in.GuildID {
		return BidView{}, ErrSelfBidNotAllowed
	}

	auction, err := s.auctions.GetByIDForUpdate(ctx, tx, activeAuction.ID)
	if err != nil {
		return BidView{}, fmt.Errorf("service: place bid: lock auction: %w", err)
	}
	if auction.Status != repository.AuctionActive {
		return BidView{}, ErrAuctionNotActive
	}

	now := time.Now().UTC()
	if now.After(auction.EndTime) {
		return BidView{}, ErrAuctionExpired
	}

	minRequired := auction.BasePrice
	highest, err := s.bids.GetHighestActiveByAuctionID(ctx, tx, auction.ID)
	switch {
	case err == nil:
		minRequired = ceilDiv(highest.Amount*minBidNumerator, minBidDenominator)
	case errors.Is(err, repository.ErrNotFound):
		// No ACTIVE bids yet: the floor is base_price.
	default:
		return BidView{}, fmt.Errorf("service: place bid: get highest bid: %w", err)
	}
	if in.Amount < minRequired {
		return BidView{}, ErrBidTooLow
	}

	reference := fmt.Sprintf("auction:%d", auction.ID)
	if err := s.goldPouches.Reserve(ctx, tx, in.GuildID, in.Amount, &reference); err != nil {
		return BidView{}, err
	}

	bid, err := s.bids.Create(ctx, tx, repository.Bid{
		AuctionID: auction.ID,
		GuildID:   in.GuildID,
		Amount:    in.Amount,
		Status:    repository.BidActive,
	})
	if err != nil {
		return BidView{}, fmt.Errorf("service: place bid: create bid: %w", err)
	}

	// Sniping protection: repeatable on every qualifying bid, since each
	// call re-locks the auction and compares "now" against whatever
	// end_time currently holds (including any prior extension).
	if auction.EndTime.Sub(now) <= bidExtensionWindow {
		auction.EndTime = auction.EndTime.Add(bidExtensionAmount)
		if err := s.auctions.Update(ctx, tx, auction); err != nil {
			return BidView{}, fmt.Errorf("service: place bid: extend auction: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return BidView{}, fmt.Errorf("service: place bid: commit: %w", err)
	}
	committed = true

	return toBidView(bid), nil
}

// CancelBid rejects the cancel if the auction isn't ACTIVE or if this bid is
// currently the highest ACTIVE bid; otherwise it releases the bid's
// reservation and marks it CANCELLED.
//
// The bid is read twice: once unlocked (just to discover which auction to
// lock), and again after the auction's row lock is held. This mirrors
// PlaceBid's locking discipline -- every mutator of a bid's Status
// (currently only CancelBid) acquires the parent auction's row lock first,
// so re-reading the bid after acquiring that lock is what makes the
// highest-bid check and the ACTIVE-status check race-free against a
// concurrent cancel or a concurrent PlaceBid on the same auction.
func (s *AuctionService) CancelBid(ctx context.Context, in CancelBidInput) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("service: cancel bid: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	initialBid, err := s.bids.GetByID(ctx, tx, in.BidID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrBidNotFound
		}
		return fmt.Errorf("service: cancel bid: get bid: %w", err)
	}

	auction, err := s.auctions.GetByIDForUpdate(ctx, tx, initialBid.AuctionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAuctionNotFound
		}
		return fmt.Errorf("service: cancel bid: lock auction: %w", err)
	}
	if auction.ItemID != in.ItemID {
		// The bid ID exists but doesn't belong to this item's auction --
		// treat identically to "not found" rather than leaking whether a
		// bid ID exists for some other item.
		return ErrBidNotFound
	}

	bid, err := s.bids.GetByID(ctx, tx, in.BidID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrBidNotFound
		}
		return fmt.Errorf("service: cancel bid: re-read bid: %w", err)
	}

	if bid.GuildID != in.GuildID {
		return ErrNotBidOwner
	}
	if auction.Status != repository.AuctionActive {
		return ErrAuctionNotActive
	}
	if bid.Status != repository.BidActive {
		return ErrBidNotActive
	}

	highest, err := s.bids.GetHighestActiveByAuctionID(ctx, tx, auction.ID)
	switch {
	case err == nil:
		if highest.ID == bid.ID {
			return ErrBidIsHighest
		}
	case errors.Is(err, repository.ErrNotFound):
		// Shouldn't happen given bid.Status == ACTIVE above, but don't
		// treat it as fatal -- there's simply nothing to compare against.
	default:
		return fmt.Errorf("service: cancel bid: get highest bid: %w", err)
	}

	reference := fmt.Sprintf("auction:%d", auction.ID)
	if err := s.goldPouches.Release(ctx, tx, bid.GuildID, bid.Amount, &reference); err != nil {
		return err
	}

	bid.Status = repository.BidCancelled
	if err := s.bids.Update(ctx, tx, bid); err != nil {
		return fmt.Errorf("service: cancel bid: update bid: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("service: cancel bid: commit: %w", err)
	}
	committed = true
	return nil
}

func (s *AuctionService) currentPrice(item repository.Item) int {
	if cached, ok := s.priceCache.Get(item.ID); ok {
		return cached
	}
	return item.Price
}

func toAuctionView(a repository.Auction) AuctionView {
	return AuctionView{
		ID:           a.ID,
		ItemID:       a.ItemID,
		OwnerGuildID: a.OwnerGuildID,
		Status:       a.Status,
		StartTime:    a.StartTime,
		EndTime:      a.EndTime,
		BasePrice:    a.BasePrice,
	}
}

func toBidView(b repository.Bid) BidView {
	return BidView{
		ID:        b.ID,
		AuctionID: b.AuctionID,
		GuildID:   b.GuildID,
		Amount:    b.Amount,
		Status:    b.Status,
		CreatedAt: b.CreatedAt,
	}
}

// ceilDiv computes ceil(a/b) for non-negative a and positive b using pure
// integer math -- used to compute "105% of the current highest bid" without
// floating point, so e.g. highest=101 correctly floors the next bid at 107
// (101*1.05=106.05, which must round UP to 107, not truncate down to 106;
// truncating division would let a 106 bid through, which is actually less
// than a full 5% increase once fractional gold isn't allowed).
func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}
