package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"DragonMarket/internal/oracle"
	"DragonMarket/internal/repository"
)

func TestItemService_CreateItem_RejectsInvalidRarity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "CreateItem InvalidRarity Forger")
	svc := newTestItemService(tx)
	qty := 5

	_, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Bad Rarity Item", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.ItemRarity("MYTHIC"), Price: new(10), Quantity: &qty,
	})
	if !errors.Is(err, ErrInvalidRarity) {
		t.Fatalf("CreateItem() error = %v, want ErrInvalidRarity", err)
	}
}

func TestItemService_CreateItem_CommonWithoutQuantity_ReturnsErrMissingQuantity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "CreateItem MissingQty Forger")
	svc := newTestItemService(tx)

	_, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "No Quantity Item", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityCommon, Price: new(10),
	})
	if !errors.Is(err, ErrMissingQuantity) {
		t.Fatalf("CreateItem() error = %v, want ErrMissingQuantity", err)
	}
}

// TestItemService_CreateItem_CommonWithoutPrice_ReturnsErrMissingPrice covers
// the bug the reviewer caught: Price used to be a plain int, so an omitted
// price field was indistinguishable from an explicit price:0, and both
// silently created a free (base_price=0) ACTIVE listing. Price is now a
// pointer (like Quantity), and both omission (nil) and an explicit 0 must be
// rejected for COMMON/RARE items.
func TestItemService_CreateItem_CommonWithoutPrice_ReturnsErrMissingPrice(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	svc := newTestItemService(tx)
	qty := 5

	cases := []struct {
		name  string
		price *int
	}{
		{"omitted", nil},
		{"explicit zero", new(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forger := createTestCharacter(t, ctx, tx, "CreateItem MissingPrice Forger "+tc.name)

			_, err := svc.CreateItem(ctx, CreateItemInput{
				Name: "No Price Item " + tc.name, LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
				Rarity: repository.RarityCommon, Price: tc.price, Quantity: &qty,
			})
			if !errors.Is(err, ErrMissingPrice) {
				t.Fatalf("CreateItem() error = %v, want ErrMissingPrice", err)
			}
		})
	}
}

func TestItemService_CreateItem_NegativePrice_ReturnsErrInvalidPrice(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "CreateItem NegativePrice Forger")
	svc := newTestItemService(tx)
	qty := 5

	_, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Negative Price Item", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityCommon, Price: new(-5), Quantity: &qty,
	})
	if !errors.Is(err, ErrInvalidPrice) {
		t.Fatalf("CreateItem() error = %v, want ErrInvalidPrice", err)
	}
}

func TestItemService_CreateItem_UnknownGuildID_ReturnsErrGuildNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "CreateItem BadGuild Forger")
	svc := newTestItemService(tx)
	qty := 3
	badGuild := int64(999999999)

	_, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Orphan Item", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityCommon, Price: new(10), Quantity: &qty, GuildID: &badGuild,
	})
	if !errors.Is(err, ErrGuildNotFound) {
		t.Fatalf("CreateItem() error = %v, want ErrGuildNotFound", err)
	}
}

func TestItemService_CreateItem_GuildIDOmitted_ResolvesToVorynthaxGuild(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	forger := createTestCharacter(t, ctx, tx, "CreateItem DefaultGuild Forger")
	svc := newTestItemService(tx)
	qty := 4

	vorynthax, err := repository.NewGuildRepository().GetByName(ctx, tx, DefaultGuildName)
	if err != nil {
		t.Fatalf("fixture: GetByName(%q) error = %v (expected seeded by migrations)", DefaultGuildName, err)
	}

	result, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Default Guild Item", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityCommon, Price: new(20), Quantity: &qty,
	})
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if result.GuildID != vorynthax.ID {
		t.Errorf("CreateItem().GuildID = %d, want %d (Vorynthax Guild)", result.GuildID, vorynthax.ID)
	}
}

func TestItemService_CreateItem_Common_CreatesInventoryAndActiveListing(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "CreateItem Common Guild")
	forger := createTestCharacter(t, ctx, tx, "CreateItem Common Forger")
	svc := newTestItemService(tx)
	qty := 10
	guildID := guild.ID

	result, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Iron Shortsword", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityCommon, Price: new(30), Quantity: &qty, GuildID: &guildID,
	})
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if result.ListingID == nil {
		t.Fatalf("CreateItem().ListingID = nil, want a listing for a COMMON item")
	}
	if result.Quantity != qty || result.GuildID != guild.ID {
		t.Errorf("CreateItem() = %+v, want Quantity=%d GuildID=%d", result, qty, guild.ID)
	}

	inv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, tx, guild.ID, result.Item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if inv.Quantity != qty {
		t.Errorf("inventory.Quantity = %d, want %d", inv.Quantity, qty)
	}

	listing, err := repository.NewListingRepository().GetByID(ctx, tx, *result.ListingID)
	if err != nil {
		t.Fatalf("GetByID(listing) error = %v", err)
	}
	if listing.Status != repository.ListingActive || listing.Quantity != qty || listing.BasePrice != 30 {
		t.Errorf("listing = %+v, want Status=ACTIVE Quantity=%d BasePrice=30", listing, qty)
	}
}

func TestItemService_CreateItem_Legendary_CreatesInventoryOnly_NoListing(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "CreateItem Legendary Guild")
	forger := createTestCharacter(t, ctx, tx, "CreateItem Legendary Forger")
	svc := newTestItemService(tx)
	guildID := guild.ID

	result, err := svc.CreateItem(ctx, CreateItemInput{
		Name: "Doombringer", LandOfOrigin: "Testland", ForgerCharacterID: forger.ID,
		Rarity: repository.RarityLegendary, Price: new(9000), GuildID: &guildID,
	})
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if result.ListingID != nil {
		t.Errorf("CreateItem().ListingID = %v, want nil for a LEGENDARY item", *result.ListingID)
	}
	if result.Quantity != 1 {
		t.Errorf("CreateItem().Quantity = %d, want 1", result.Quantity)
	}
	if !result.Item.IsLegendary {
		t.Errorf("CreateItem().Item.IsLegendary = false, want true")
	}

	inv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, tx, guild.ID, result.Item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if inv.Quantity != 1 {
		t.Errorf("inventory.Quantity = %d, want 1", inv.Quantity)
	}

	listings, err := repository.NewListingRepository().ListByStatus(ctx, tx, repository.ListingActive)
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	for _, l := range listings {
		if l.ItemID == result.Item.ID {
			t.Errorf("found an ACTIVE listing %+v for a LEGENDARY item, want none", l)
		}
	}
}

func TestItemService_GetItem_NotFound_ReturnsErrItemNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	svc := newTestItemService(tx)

	_, err := svc.GetItem(ctx, 999999999)
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("GetItem() error = %v, want ErrItemNotFound", err)
	}
}

func TestItemService_GetItem_Legendary_UsesCachePrice_FallsBackToDBPrice(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "GetItem Legendary Guild")
	forger := createTestCharacter(t, ctx, tx, "GetItem Legendary Forger")

	item, err := repository.NewItemRepository().Create(ctx, tx, repository.Item{
		Name: "Cached Blade", LandOfOrigin: "Testland", Rarity: repository.RarityLegendary,
		ForgerCharacterID: forger.ID, Price: 8000,
	})
	if err != nil {
		t.Fatalf("fixture: create item: %v", err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, tx, repository.Inventory{
		GuildID: guild.ID, ItemID: item.ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("fixture: create inventory: %v", err)
	}

	cache := oracle.NewCache()
	svc := NewItemService(
		tx, repository.NewItemRepository(), repository.NewListingRepository(),
		repository.NewInventoryRepository(), repository.NewGuildRepository(), newTestService(), cache,
	)

	// Before the oracle's first tick, the cache has no entry yet: GetItem
	// must fall back to the DB-stored Item.Price rather than e.g. zero.
	beforeCache, err := svc.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if beforeCache.Price != 8000 {
		t.Errorf("GetItem().Price (no cache entry) = %d, want 8000 (DB fallback)", beforeCache.Price)
	}
	if !beforeCache.IsLegendary {
		t.Errorf("GetItem().IsLegendary = false, want true")
	}

	cache.Set(item.ID, 8500)
	afterCache, err := svc.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if afterCache.Price != 8500 {
		t.Errorf("GetItem().Price (cache entry present) = %d, want 8500 (cache, not DB read)", afterCache.Price)
	}
}

func TestItemService_ListItems_IncludesLegendaryFlag(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "ListItems Guild")
	svc := newTestItemService(tx)

	commonItem, _ := createTestListedItem(t, ctx, tx, "ListItems Common", repository.RarityCommon, 10, 5, guild)
	forger := createTestCharacter(t, ctx, tx, "ListItems Legendary Forger")
	legendaryItem, err := repository.NewItemRepository().Create(ctx, tx, repository.Item{
		Name: "ListItems Legendary", LandOfOrigin: "Testland", Rarity: repository.RarityLegendary,
		ForgerCharacterID: forger.ID, Price: 7000,
	})
	if err != nil {
		t.Fatalf("fixture: create legendary item: %v", err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, tx, repository.Inventory{
		GuildID: guild.ID, ItemID: legendaryItem.ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("fixture: create legendary inventory: %v", err)
	}

	items, err := svc.ListItems(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}

	var foundCommon, foundLegendary bool
	for _, it := range items {
		switch it.ID {
		case commonItem.ID:
			foundCommon = true
			if it.IsLegendary {
				t.Errorf("common item %d marked IsLegendary=true", it.ID)
			}
		case legendaryItem.ID:
			foundLegendary = true
			if !it.IsLegendary {
				t.Errorf("legendary item %d marked IsLegendary=false", it.ID)
			}
			if it.Price != 7000 {
				t.Errorf("legendary item %d Price = %d, want 7000 (DB fallback, no cache entry)", it.ID, it.Price)
			}
		}
	}
	if !foundCommon || !foundLegendary {
		t.Fatalf("ListItems() missing fixtures: foundCommon=%v foundLegendary=%v", foundCommon, foundLegendary)
	}
}

func TestItemService_PurchaseItem_HappyPath_SettlesBothSidesAndDecrementsQuantities(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	sellerGuild := createTestGuild(t, ctx, tx, "Purchase Seller Guild")
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase Buyer Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID: sellerGuild.ID, TotalBalance: 0, DailySpendingLimit: 100000,
	})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{
		GuildID: buyerGuild.ID, TotalBalance: 1000, DailySpendingLimit: 100000,
	})
	item, listing := createTestListedItem(t, ctx, tx, "Purchase Happy Item", repository.RarityCommon, 25, 10, sellerGuild)
	svc := newTestItemService(tx)

	result, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 4})
	if err != nil {
		t.Fatalf("PurchaseItem() error = %v", err)
	}
	if result.TotalPrice != 100 || result.UnitPrice != 25 || result.Quantity != 4 {
		t.Errorf("PurchaseItem() = %+v, want TotalPrice=100 UnitPrice=25 Quantity=4", result)
	}
	if result.ListingStatus != repository.ListingActive {
		t.Errorf("PurchaseItem().ListingStatus = %q, want ACTIVE (6 remaining)", result.ListingStatus)
	}

	updatedListing, err := repository.NewListingRepository().GetByID(ctx, tx, listing.ID)
	if err != nil {
		t.Fatalf("GetByID(listing) error = %v", err)
	}
	if updatedListing.Quantity != 6 {
		t.Errorf("listing.Quantity = %d, want 6", updatedListing.Quantity)
	}

	inv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, tx, sellerGuild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if inv.Quantity != 6 {
		t.Errorf("inventory.Quantity = %d, want 6", inv.Quantity)
	}

	buyerPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, buyerGuild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID(buyer) error = %v", err)
	}
	if buyerPouch.TotalBalance != 900 {
		t.Errorf("buyer TotalBalance = %d, want 900", buyerPouch.TotalBalance)
	}
	if buyerPouch.ReservedBalance != 0 {
		t.Errorf("buyer ReservedBalance = %d, want 0 (reserve+settle must net to zero reserved)", buyerPouch.ReservedBalance)
	}

	sellerPouch, err := repository.NewGoldPouchRepository().GetByGuildID(ctx, tx, sellerGuild.ID)
	if err != nil {
		t.Fatalf("GetByGuildID(seller) error = %v", err)
	}
	if sellerPouch.TotalBalance != 100 {
		t.Errorf("seller TotalBalance = %d, want 100", sellerPouch.TotalBalance)
	}

	buyerLogs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, buyerGuild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID(buyer) error = %v", err)
	}
	var sawPurchase bool
	for _, l := range buyerLogs {
		if l.Type == repository.TxPurchase && l.Amount == 100 {
			sawPurchase = true
		}
	}
	if !sawPurchase {
		t.Errorf("buyer transaction_logs = %+v, want a PURCHASE entry for 100", buyerLogs)
	}

	sellerLogs, err := repository.NewTransactionLogRepository().ListByGuildID(ctx, tx, sellerGuild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID(seller) error = %v", err)
	}
	var sawCredit bool
	for _, l := range sellerLogs {
		if l.Type == repository.TxCredit && l.Amount == 100 {
			sawCredit = true
		}
	}
	if !sawCredit {
		t.Errorf("seller transaction_logs = %+v, want a CREDIT entry for 100", sellerLogs)
	}
}

func TestItemService_PurchaseItem_ExactRemainingQuantity_ExpiresListingAtomically(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	sellerGuild := createTestGuild(t, ctx, tx, "Purchase Expire Seller Guild")
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase Expire Buyer Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: sellerGuild.ID, TotalBalance: 0, DailySpendingLimit: 100000})
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: buyerGuild.ID, TotalBalance: 500, DailySpendingLimit: 100000})
	item, listing := createTestListedItem(t, ctx, tx, "Purchase Expire Item", repository.RarityRare, 50, 2, sellerGuild)
	svc := newTestItemService(tx)

	result, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 2})
	if err != nil {
		t.Fatalf("PurchaseItem() error = %v", err)
	}
	if result.ListingStatus != repository.ListingExpired {
		t.Errorf("PurchaseItem().ListingStatus = %q, want EXPIRED", result.ListingStatus)
	}

	updatedListing, err := repository.NewListingRepository().GetByID(ctx, tx, listing.ID)
	if err != nil {
		t.Fatalf("GetByID(listing) error = %v", err)
	}
	if updatedListing.Quantity != 0 || updatedListing.Status != repository.ListingExpired {
		t.Errorf("listing = %+v, want Quantity=0 Status=EXPIRED", updatedListing)
	}
}

func TestItemService_PurchaseItem_InsufficientQuantity_ReturnsErrInsufficientQuantity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	sellerGuild := createTestGuild(t, ctx, tx, "Purchase InsufficientQty Seller Guild")
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase InsufficientQty Buyer Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: buyerGuild.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	item, _ := createTestListedItem(t, ctx, tx, "Purchase InsufficientQty Item", repository.RarityCommon, 10, 3, sellerGuild)
	svc := newTestItemService(tx)

	_, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 4})
	if !errors.Is(err, ErrInsufficientQuantity) {
		t.Fatalf("PurchaseItem() error = %v, want ErrInsufficientQuantity", err)
	}
}

func TestItemService_PurchaseItem_LegendaryItem_ReturnsErrListingNotActive(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "Purchase Legendary Guild")
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase Legendary Buyer Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: buyerGuild.ID, TotalBalance: 100000, DailySpendingLimit: 1000000})
	forger := createTestCharacter(t, ctx, tx, "Purchase Legendary Forger")
	item, err := repository.NewItemRepository().Create(ctx, tx, repository.Item{
		Name: "Purchase Legendary Item", LandOfOrigin: "Testland", Rarity: repository.RarityLegendary,
		ForgerCharacterID: forger.ID, Price: 9000,
	})
	if err != nil {
		t.Fatalf("fixture: create item: %v", err)
	}
	if _, err := repository.NewInventoryRepository().Create(ctx, tx, repository.Inventory{
		GuildID: guild.ID, ItemID: item.ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("fixture: create inventory: %v", err)
	}
	svc := newTestItemService(tx)

	_, err = svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 1})
	if !errors.Is(err, ErrListingNotActive) {
		t.Fatalf("PurchaseItem() error = %v, want ErrListingNotActive", err)
	}
}

func TestItemService_PurchaseItem_ItemNotFound_ReturnsErrItemNotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase NotFound Buyer Guild")
	svc := newTestItemService(tx)

	_, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: 999999999, BuyerGuildID: buyerGuild.ID, Quantity: 1})
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("PurchaseItem() error = %v, want ErrItemNotFound", err)
	}
}

func TestItemService_PurchaseItem_RejectsNonPositiveQuantity(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase BadQty Buyer Guild")
	sellerGuild := createTestGuild(t, ctx, tx, "Purchase BadQty Seller Guild")
	item, _ := createTestListedItem(t, ctx, tx, "Purchase BadQty Item", repository.RarityCommon, 10, 5, sellerGuild)
	svc := newTestItemService(tx)

	_, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 0})
	if !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("PurchaseItem() quantity=0 error = %v, want ErrInvalidQuantity", err)
	}
}

func TestItemService_PurchaseItem_InsufficientBalance_ReturnsErrInsufficientBalance(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	sellerGuild := createTestGuild(t, ctx, tx, "Purchase PoorBuyer Seller Guild")
	buyerGuild := createTestGuild(t, ctx, tx, "Purchase PoorBuyer Buyer Guild")
	createTestGoldPouch(t, ctx, tx, repository.GoldPouch{GuildID: buyerGuild.ID, TotalBalance: 10, DailySpendingLimit: 100000})
	item, _ := createTestListedItem(t, ctx, tx, "Purchase PoorBuyer Item", repository.RarityCommon, 50, 5, sellerGuild)
	svc := newTestItemService(tx)

	_, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyerGuild.ID, Quantity: 1})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("PurchaseItem() error = %v, want ErrInsufficientBalance", err)
	}

	listing, err := repository.NewListingRepository().GetActiveByItemID(ctx, tx, item.ID)
	if err != nil {
		t.Fatalf("GetActiveByItemID() error = %v", err)
	}
	if listing.Quantity != 5 {
		t.Errorf("listing.Quantity after rejected purchase = %d, want 5 (unchanged)", listing.Quantity)
	}
}

func TestItemService_PurchaseItem_ConcurrentPurchasesNeverOversell(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	// Fixtures are committed directly against testPool (see
	// TestGoldPouchService_Reserve_ConcurrentReservationsSerializeCorrectly's
	// doc comment in goldpouch_service_test.go for why), so names must be
	// unique per invocation to survive `go test -count>1` in the same
	// process.
	sellerGuild := createTestGuild(t, ctx, testPool, fmt.Sprintf("Concurrent Purchase Seller %d", suffix))
	buyerGuildA := createTestGuild(t, ctx, testPool, fmt.Sprintf("Concurrent Purchase BuyerA %d", suffix))
	buyerGuildB := createTestGuild(t, ctx, testPool, fmt.Sprintf("Concurrent Purchase BuyerB %d", suffix))
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: sellerGuild.ID, TotalBalance: 0, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: buyerGuildA.ID, TotalBalance: 1000, DailySpendingLimit: 1000000})
	createTestGoldPouch(t, ctx, testPool, repository.GoldPouch{GuildID: buyerGuildB.ID, TotalBalance: 1000, DailySpendingLimit: 1000000})

	item, _ := createTestListedItem(t, ctx, testPool, fmt.Sprintf("Concurrent Purchase Item %d", suffix), repository.RarityCommon, 100, 1, sellerGuild)

	buyers := []int64{buyerGuildA.ID, buyerGuildB.ID}
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := newTestItemService(testPool)
			_, err := svc.PurchaseItem(ctx, PurchaseInput{ItemID: item.ID, BuyerGuildID: buyers[i], Quantity: 1})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInsufficientQuantity), errors.Is(err, ErrListingNotActive):
			failures++
		default:
			t.Fatalf("goroutine returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d, want 1", successes)
	}
	if failures != 1 {
		t.Errorf("failures = %d, want 1", failures)
	}

	finalInv, err := repository.NewInventoryRepository().GetByGuildAndItem(ctx, testPool, sellerGuild.ID, item.ID)
	if err != nil {
		t.Fatalf("GetByGuildAndItem() error = %v", err)
	}
	if finalInv.Quantity != 0 {
		t.Errorf("final inventory.Quantity = %d, want 0 (never oversold)", finalInv.Quantity)
	}

	listings, err := repository.NewListingRepository().ListByStatus(ctx, testPool, repository.ListingExpired)
	if err != nil {
		t.Fatalf("ListByStatus(EXPIRED) error = %v", err)
	}
	var foundExpired bool
	for _, l := range listings {
		if l.ItemID == item.ID {
			foundExpired = true
			if l.Quantity != 0 {
				t.Errorf("final listing.Quantity = %d, want 0", l.Quantity)
			}
		}
	}
	if !foundExpired {
		t.Errorf("expected the listing for item %d to have flipped to EXPIRED", item.ID)
	}
}
