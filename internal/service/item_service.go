package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

const DefaultGuildName = "Vorynthax Guild"

var (
	ErrInvalidRarity        = errors.New("service: rarity must be COMMON, RARE, or LEGENDARY")
	ErrInvalidPrice         = errors.New("service: price must be non-negative")
	ErrMissingPrice         = errors.New("service: price is required and must be positive for COMMON/RARE items")
	ErrMissingQuantity      = errors.New("service: quantity is required and must be positive for COMMON/RARE items")
	ErrGuildNotFound        = errors.New("service: guild not found")
	ErrItemNotFound         = errors.New("service: item not found")
	ErrListingNotActive     = errors.New("service: item has no active listing")
	ErrInsufficientQuantity = errors.New("service: listing does not have enough quantity remaining")
	ErrInvalidQuantity      = errors.New("service: purchase quantity must be positive")
	ErrLegendaryConflict    = errors.New("service: legendary item ownership conflict")
)

const (
	pgErrCodeUniqueViolation = "23505"
	pgErrCodeCheckViolation  = "23514"
)

const defaultListLimit = 100

type TxPool interface {
	repository.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type ItemService struct {
	db          TxPool
	items       repository.ItemRepository
	listings    repository.ListingRepository
	inventories repository.InventoryRepository
	guilds      repository.GuildRepository
	goldPouches *GoldPouchService
	priceCache  *oracle.Cache
}

func NewItemService(
	db TxPool,
	items repository.ItemRepository,
	listings repository.ListingRepository,
	inventories repository.InventoryRepository,
	guilds repository.GuildRepository,
	goldPouches *GoldPouchService,
	priceCache *oracle.Cache,
) *ItemService {
	return &ItemService{
		db:          db,
		items:       items,
		listings:    listings,
		inventories: inventories,
		guilds:      guilds,
		goldPouches: goldPouches,
		priceCache:  priceCache,
	}
}

type CreateItemInput struct {
	Name              string
	LandOfOrigin      string
	ForgerCharacterID int64
	Rarity            repository.ItemRarity
	// Price is a pointer (like Quantity) so an omitted request field is
	// distinguishable from an explicit price:0 -- both must be rejected for
	// COMMON/RARE items (see ErrMissingPrice), rather than the former
	// silently creating a free ACTIVE listing.
	Price    *int
	GuildID  *int64 // nil => resolves to DefaultGuildName
	Quantity *int
}

type ItemView struct {
	ID                int64
	Name              string
	LandOfOrigin      string
	Rarity            repository.ItemRarity
	ForgerCharacterID int64
	Price             int
	IsLegendary       bool
}

type CreateItemResult struct {
	Item      ItemView
	GuildID   int64
	Quantity  int
	ListingID *int64 // nil for LEGENDARY items (no listing/auction is created)
}

type PurchaseInput struct {
	ItemID       int64
	BuyerGuildID int64
	Quantity     int
}

type PurchaseResult struct {
	ItemID        int64
	Quantity      int
	UnitPrice     int
	TotalPrice    int
	SellerGuildID int64
	ListingID     int64
	ListingStatus repository.ListingStatus
}

func (s *ItemService) CreateItem(ctx context.Context, in CreateItemInput) (CreateItemResult, error) {
	switch in.Rarity {
	case repository.RarityCommon, repository.RarityRare, repository.RarityLegendary:
	default:
		return CreateItemResult{}, ErrInvalidRarity
	}
	isListable := in.Rarity == repository.RarityCommon || in.Rarity == repository.RarityRare

	if in.Price == nil {
		return CreateItemResult{}, ErrMissingPrice
	}
	price := *in.Price
	if price < 0 {
		return CreateItemResult{}, ErrInvalidPrice
	}
	// A zero base_price would create a free ACTIVE listing for a
	// COMMON/RARE item, which the plan's "request must include quantity
	// and price" requirement rules out just as much as omitting the field
	// entirely -- treat both as ErrMissingPrice. LEGENDARY items don't get
	// a listing, so a zero starting price (before the oracle's first tick
	// overwrites it) isn't a marketplace-safety issue the same way.
	if isListable && price == 0 {
		return CreateItemResult{}, ErrMissingPrice
	}

	if isListable && (in.Quantity == nil || *in.Quantity <= 0) {
		return CreateItemResult{}, ErrMissingQuantity
	}

	guildID, err := s.resolveGuildID(ctx, s.db, in.GuildID)
	if err != nil {
		return CreateItemResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreateItemResult{}, fmt.Errorf("service: create item: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	item, err := s.items.Create(ctx, tx, repository.Item{
		Name:              in.Name,
		LandOfOrigin:      in.LandOfOrigin,
		Rarity:            in.Rarity,
		ForgerCharacterID: in.ForgerCharacterID,
		Price:             price,
	})
	if err != nil {
		if isConflictErr(err) {
			return CreateItemResult{}, ErrLegendaryConflict
		}
		return CreateItemResult{}, fmt.Errorf("service: create item: %w", err)
	}

	invQuantity := 1
	if isListable {
		invQuantity = *in.Quantity
	}
	if _, err := s.inventories.Create(ctx, tx, repository.Inventory{
		GuildID: guildID, ItemID: item.ID, Quantity: invQuantity,
	}); err != nil {
		if isConflictErr(err) {
			return CreateItemResult{}, ErrLegendaryConflict
		}
		return CreateItemResult{}, fmt.Errorf("service: create item: create inventory: %w", err)
	}

	var listingID *int64
	if isListable {
		l, err := s.listings.Create(ctx, tx, repository.Listing{
			ItemID:    item.ID,
			GuildID:   guildID,
			Quantity:  invQuantity,
			BasePrice: price,
			Status:    repository.ListingActive,
		})
		if err != nil {
			return CreateItemResult{}, fmt.Errorf("service: create item: create listing: %w", err)
		}
		listingID = &l.ID
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateItemResult{}, fmt.Errorf("service: create item: commit: %w", err)
	}
	committed = true

	return CreateItemResult{
		Item:      s.toView(item),
		GuildID:   guildID,
		Quantity:  invQuantity,
		ListingID: listingID,
	}, nil
}

func (s *ItemService) GetItem(ctx context.Context, id int64) (ItemView, error) {
	item, err := s.items.GetByID(ctx, s.db, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ItemView{}, ErrItemNotFound
		}
		return ItemView{}, fmt.Errorf("service: get item: %w", err)
	}
	return s.toView(item), nil
}

func (s *ItemService) ListItems(ctx context.Context, limit, offset int) ([]ItemView, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.items.List(ctx, s.db, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("service: list items: %w", err)
	}
	views := make([]ItemView, len(items))
	for i, item := range items {
		views[i] = s.toView(item)
	}
	return views, nil
}

func (s *ItemService) PurchaseItem(ctx context.Context, in PurchaseInput) (PurchaseResult, error) {
	if in.Quantity <= 0 {
		return PurchaseResult{}, ErrInvalidQuantity
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: begin tx: %w", err)
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
			return PurchaseResult{}, ErrItemNotFound
		}
		return PurchaseResult{}, fmt.Errorf("service: purchase: get item: %w", err)
	}
	if item.Rarity != repository.RarityCommon && item.Rarity != repository.RarityRare {
		return PurchaseResult{}, ErrListingNotActive
	}

	activeListing, err := s.listings.GetActiveByItemID(ctx, tx, in.ItemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return PurchaseResult{}, ErrListingNotActive
		}
		return PurchaseResult{}, fmt.Errorf("service: purchase: get active listing: %w", err)
	}

	// Lock the listing AND the seller's inventory row before checking
	// anything: per the Global Constraints concurrency rule, every
	// read-then-write of a row involved in a financial/ownership change
	// must be locked first. Both locks are acquired up front, then both
	// checks run against the locked data -- this avoids a TOCTOU window
	// where a quantity check runs against one row while the other is still
	// unlocked (harmless today since nothing mutates inventory
	// independently of its paired listing yet, but future-proofs against a
	// task that adds one).
	listing, err := s.listings.GetByIDForUpdate(ctx, tx, activeListing.ID)
	if err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: lock listing: %w", err)
	}
	inv, err := s.inventories.GetByGuildAndItemForUpdate(ctx, tx, listing.GuildID, in.ItemID)
	if err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: lock inventory: %w", err)
	}

	if listing.Status != repository.ListingActive {
		return PurchaseResult{}, ErrListingNotActive
	}
	if listing.Quantity < in.Quantity {
		return PurchaseResult{}, ErrInsufficientQuantity
	}
	if inv.Quantity < in.Quantity {
		return PurchaseResult{}, ErrInsufficientQuantity
	}

	total := listing.BasePrice * in.Quantity
	reference := fmt.Sprintf("listing:%d", listing.ID)

	if err := s.goldPouches.Reserve(ctx, tx, in.BuyerGuildID, total, &reference); err != nil {
		return PurchaseResult{}, err
	}
	if err := s.goldPouches.Settle(ctx, tx, in.BuyerGuildID, total, repository.TxPurchase, &reference); err != nil {
		return PurchaseResult{}, err
	}
	if err := s.goldPouches.Credit(ctx, tx, listing.GuildID, total, &reference); err != nil {
		return PurchaseResult{}, err
	}

	newQuantity := listing.Quantity - in.Quantity
	newStatus := listing.Status
	if newQuantity == 0 {
		newStatus = repository.ListingExpired
	}
	listing.Quantity = newQuantity
	listing.Status = newStatus
	if err := s.listings.Update(ctx, tx, listing); err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: update listing: %w", err)
	}

	if err := s.inventories.UpdateQuantity(ctx, tx, listing.GuildID, in.ItemID, inv.Quantity-in.Quantity); err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: update inventory: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return PurchaseResult{}, fmt.Errorf("service: purchase: commit: %w", err)
	}
	committed = true

	return PurchaseResult{
		ItemID:        in.ItemID,
		Quantity:      in.Quantity,
		UnitPrice:     listing.BasePrice,
		TotalPrice:    total,
		SellerGuildID: listing.GuildID,
		ListingID:     listing.ID,
		ListingStatus: newStatus,
	}, nil
}

func (s *ItemService) resolveGuildID(ctx context.Context, db repository.DBTX, guildID *int64) (int64, error) {
	if guildID != nil {
		g, err := s.guilds.GetByID(ctx, db, *guildID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return 0, ErrGuildNotFound
			}
			return 0, fmt.Errorf("service: resolve guild: %w", err)
		}
		return g.ID, nil
	}

	g, err := s.guilds.GetByName(ctx, db, DefaultGuildName)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrGuildNotFound
		}
		return 0, fmt.Errorf("service: resolve default guild: %w", err)
	}
	return g.ID, nil
}

func (s *ItemService) toView(item repository.Item) ItemView {
	price := item.Price
	legendary := item.Rarity == repository.RarityLegendary
	if legendary {
		if cached, ok := s.priceCache.Get(item.ID); ok {
			price = cached
		}
	}
	return ItemView{
		ID:                item.ID,
		Name:              item.Name,
		LandOfOrigin:      item.LandOfOrigin,
		Rarity:            item.Rarity,
		ForgerCharacterID: item.ForgerCharacterID,
		Price:             price,
		IsLegendary:       legendary,
	}
}

func isConflictErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgErrCodeUniqueViolation, pgErrCodeCheckViolation:
			return true
		}
	}
	return false
}
