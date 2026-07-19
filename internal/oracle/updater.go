package oracle

import (
	"context"
	"log/slog"
	"time"

	"DragonMarket/internal/repository"
)

const defaultPerItemTimeout = 2 * time.Second

type ItemPriceStore interface {
	ListByRarity(ctx context.Context, db repository.DBTX, rarity repository.ItemRarity) ([]repository.Item, error)
	UpdatePrice(ctx context.Context, db repository.DBTX, id int64, price int) error
}

// Compile-time check that the real repository satisfies ItemPriceStore.
var _ ItemPriceStore = (repository.ItemRepository)(nil)

type Updater struct {
	Oracle         PriceOracleService
	Cache          *Cache
	Items          ItemPriceStore
	DB             repository.DBTX
	Interval       time.Duration
	PerItemTimeout time.Duration
	Logger         *slog.Logger

	// OnTick, if set, is invoked after every tick with the error Tick
	// returned (nil on success). It exists purely so tests can observe how
	// many ticks Run() performed without sleeping for real intervals.
	OnTick func(err error)
}

func NewUpdater(oracle PriceOracleService, cache *Cache, items ItemPriceStore, db repository.DBTX, interval time.Duration) *Updater {
	return &Updater{
		Oracle:         oracle,
		Cache:          cache,
		Items:          items,
		DB:             db,
		Interval:       interval,
		PerItemTimeout: defaultPerItemTimeout,
	}
}

func (u *Updater) Tick(ctx context.Context) error {
	items, err := u.Items.ListByRarity(ctx, u.DB, repository.RarityLegendary)
	if err != nil {
		return err
	}

	for _, item := range items {
		u.refreshItem(ctx, item.ID)
	}
	return nil
}

func (u *Updater) refreshItem(ctx context.Context, itemID int64) {
	timeout := u.PerItemTimeout
	if timeout <= 0 {
		timeout = defaultPerItemTimeout
	}
	itemCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	price, err := u.Oracle.GetPrice(itemCtx, itemID)
	if err != nil {
		u.logger().Warn("price oracle call failed, keeping last known-good price",
			"item_id", itemID, "error", err)
		return
	}
	if price <= 0 {
		u.logger().Warn("price oracle returned an invalid price, keeping last known-good price",
			"item_id", itemID, "price", price)
		return
	}

	if err := u.Items.UpdatePrice(ctx, u.DB, itemID, price); err != nil {
		u.logger().Warn("failed to persist oracle price, keeping last known-good price",
			"item_id", itemID, "error", err)
		return
	}

	u.Cache.Set(itemID, price)
}

func (u *Updater) logger() *slog.Logger {
	if u.Logger != nil {
		return u.Logger
	}
	return slog.Default()
}

// Run starts the ticker loop: every Interval it performs one Tick, until ctx
// is canceled, at which point Run returns. It is meant to be launched in its
// own goroutine (e.g. `go updater.Run(ctx)`), per the project's scope of a
// context.Context + time.Ticker rather than a general job-scheduling
// framework.
func (u *Updater) Run(ctx context.Context) {
	ticker := time.NewTicker(u.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := u.Tick(ctx)
			if err != nil {
				u.logger().Warn("oracle tick failed", "error", err)
			}
			if u.OnTick != nil {
				u.OnTick(err)
			}
		}
	}
}
