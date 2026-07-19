package oracle_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

type fakeItemStore struct {
	mu            sync.Mutex
	items         []repository.Item
	listErr       error
	updateErr     map[int64]error
	updatedPrices map[int64]int
	updateCalls   int
}

func newFakeItemStore(items ...repository.Item) *fakeItemStore {
	return &fakeItemStore{
		items:         items,
		updateErr:     make(map[int64]error),
		updatedPrices: make(map[int64]int),
	}
}

func (f *fakeItemStore) ListByRarity(ctx context.Context, db repository.DBTX, rarity repository.ItemRarity) ([]repository.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]repository.Item, len(f.items))
	copy(out, f.items)
	return out, nil
}

func (f *fakeItemStore) UpdatePrice(ctx context.Context, db repository.DBTX, id int64, price int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	if err, ok := f.updateErr[id]; ok {
		return err
	}
	f.updatedPrices[id] = price
	return nil
}

func (f *fakeItemStore) priceFor(id int64) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.updatedPrices[id]
	return p, ok
}

func legendaryItem(id int64) repository.Item {
	return repository.Item{ID: id, Name: "Test Legendary", Rarity: repository.RarityLegendary, Price: 1}
}

func TestUpdater_Tick_SuccessUpdatesCacheAndRepo(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 5000})
	cache := oracle.NewCache()

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	gotPrice, ok := store.priceFor(1)
	if !ok || gotPrice != 5000 {
		t.Errorf("repo price for item 1 = (%d, %v), want (5000, true)", gotPrice, ok)
	}
	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 5000 {
		t.Errorf("cache price for item 1 = (%d, %v), want (5000, true)", cachedPrice, ok)
	}
}

func TestUpdater_Tick_OracleErrorLeavesCacheAndRepoUntouched(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Err: errors.New("oracle down")})
	cache := oracle.NewCache()
	cache.Set(1, 42) // pre-existing known-good price must survive

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (per-item errors are logged, not returned)", err)
	}

	if _, ok := store.priceFor(1); ok {
		t.Errorf("repo price for item 1 was updated, want untouched on oracle error")
	}
	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 42 {
		t.Errorf("cache price for item 1 = (%d, %v), want (42, true) - untouched", cachedPrice, ok)
	}
}

func TestUpdater_Tick_OracleTimeoutLeavesCacheAndRepoUntouched(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 999, Delay: 200 * time.Millisecond})
	cache := oracle.NewCache()
	cache.Set(1, 42)

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)
	u.PerItemTimeout = 5 * time.Millisecond

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	if _, ok := store.priceFor(1); ok {
		t.Errorf("repo price for item 1 was updated, want untouched on oracle timeout")
	}
	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 42 {
		t.Errorf("cache price for item 1 = (%d, %v), want (42, true) - untouched", cachedPrice, ok)
	}
}

func TestUpdater_Tick_ZeroPriceLeavesCacheAndRepoUntouched(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 0})
	cache := oracle.NewCache()
	cache.Set(1, 42)

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	if _, ok := store.priceFor(1); ok {
		t.Errorf("repo price for item 1 was updated, want untouched on zero price")
	}
	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 42 {
		t.Errorf("cache price for item 1 = (%d, %v), want (42, true) - untouched", cachedPrice, ok)
	}
}

func TestUpdater_Tick_NegativePriceLeavesCacheAndRepoUntouched(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: -10})
	cache := oracle.NewCache()
	cache.Set(1, 42)

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	if _, ok := store.priceFor(1); ok {
		t.Errorf("repo price for item 1 was updated, want untouched on negative price")
	}
	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 42 {
		t.Errorf("cache price for item 1 = (%d, %v), want (42, true) - untouched", cachedPrice, ok)
	}
}

func TestUpdater_Tick_RepoUpdateFailureLeavesCacheUntouched(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	store.updateErr[1] = errors.New("db write failed")
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 5000})
	cache := oracle.NewCache()
	cache.Set(1, 42)

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)

	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	cachedPrice, ok := cache.Get(1)
	if !ok || cachedPrice != 42 {
		t.Errorf("cache price for item 1 = (%d, %v), want (42, true) - untouched when DB write fails", cachedPrice, ok)
	}
}

func TestUpdater_Tick_MixedResultsPerItem(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1), legendaryItem(2), legendaryItem(3))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 100})
	mockOracle.SetForItem(2, oracle.MockResponse{Err: errors.New("boom")})
	mockOracle.SetForItem(3, oracle.MockResponse{Price: -5})
	cache := oracle.NewCache()

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)
	if err := u.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	if p, ok := cache.Get(1); !ok || p != 100 {
		t.Errorf("item 1 cache = (%d,%v), want (100,true)", p, ok)
	}
	if _, ok := cache.Get(2); ok {
		t.Errorf("item 2 cache should be untouched (oracle error)")
	}
	if _, ok := cache.Get(3); ok {
		t.Errorf("item 3 cache should be untouched (negative price)")
	}
}

func TestUpdater_Tick_ListErrorReturnsError(t *testing.T) {
	store := newFakeItemStore()
	store.listErr = errors.New("db unreachable")
	mockOracle := oracle.NewMockPriceOracleService()
	cache := oracle.NewCache()

	u := oracle.NewUpdater(mockOracle, cache, store, nil, time.Second)
	if err := u.Tick(context.Background()); !errors.Is(err, store.listErr) {
		t.Fatalf("Tick() error = %v, want %v", err, store.listErr)
	}
}

func TestUpdater_Run_TicksRepeatedlyUntilContextCanceled(t *testing.T) {
	store := newFakeItemStore(legendaryItem(1))
	mockOracle := oracle.NewMockPriceOracleService()
	mockOracle.SetDefault(oracle.MockResponse{Price: 10})
	cache := oracle.NewCache()

	u := oracle.NewUpdater(mockOracle, cache, store, nil, 5*time.Millisecond)

	var tickCount int32
	u.OnTick = func(error) { atomic.AddInt32(&tickCount, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		u.Run(ctx)
		close(done)
	}()

	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	if atomic.LoadInt32(&tickCount) < 2 {
		t.Errorf("tickCount = %d, want at least 2 ticks in 60ms at a 5ms interval", tickCount)
	}
}
