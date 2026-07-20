package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type fakeItemService struct {
	createResult service.CreateItemResult
	createErr    error
	lastCreate   service.CreateItemInput

	items   []service.ItemView
	listErr error
	getView service.ItemView
	getErr  error
	lastGet int64

	purchaseResult service.PurchaseResult
	purchaseErr    error
	lastPurchase   service.PurchaseInput
}

func (f *fakeItemService) CreateItem(ctx context.Context, in service.CreateItemInput) (service.CreateItemResult, error) {
	f.lastCreate = in
	return f.createResult, f.createErr
}

func (f *fakeItemService) GetItem(ctx context.Context, id int64) (service.ItemView, error) {
	f.lastGet = id
	return f.getView, f.getErr
}

func (f *fakeItemService) ListItems(ctx context.Context, limit, offset int) ([]service.ItemView, error) {
	return f.items, f.listErr
}

func (f *fakeItemService) PurchaseItem(ctx context.Context, in service.PurchaseInput) (service.PurchaseResult, error) {
	f.lastPurchase = in
	return f.purchaseResult, f.purchaseErr
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v (body=%s)", err, rec.Body.String())
	}
	return env
}

func routerWithFake(fake *fakeItemService) *gin.Engine {
	h := NewItemHandlers(fake)
	router := gin.New()
	router.POST("/items", h.Create)
	router.GET("/items", h.List)
	router.GET("/items/:id", h.Get)
	router.POST("/items/:id/purchase", h.Purchase)
	return router
}

func TestItemHandlers_Create_InvalidJSON_Returns400(t *testing.T) {
	fake := &fakeItemService{}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", env.Error.Code)
	}
}

func TestItemHandlers_Create_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid rarity", service.ErrInvalidRarity, http.StatusBadRequest, "INVALID_RARITY"},
		{"missing quantity", service.ErrMissingQuantity, http.StatusBadRequest, "MISSING_QUANTITY"},
		{"missing price", service.ErrMissingPrice, http.StatusBadRequest, "MISSING_PRICE"},
		{"invalid price", service.ErrInvalidPrice, http.StatusBadRequest, "INVALID_PRICE"},
		{"guild not found", service.ErrGuildNotFound, http.StatusNotFound, "GUILD_NOT_FOUND"},
		{"legendary conflict", service.ErrLegendaryConflict, http.StatusConflict, "LEGENDARY_CONFLICT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeItemService{createErr: tc.err}
			router := routerWithFake(fake)

			body := `{"name":"Sword","land_of_origin":"Testland","forger_character_id":1,"rarity":"COMMON","price":10,"quantity":5}`
			req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			env := decodeErrorBody(t, rec)
			if env.Error.Code != tc.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestItemHandlers_Create_Success_PassesRequestFieldsAndReturns201(t *testing.T) {
	guildID := int64(7)
	qty := 5
	fake := &fakeItemService{
		createResult: service.CreateItemResult{
			Item: service.ItemView{
				ID: 42, Name: "Iron Sword", LandOfOrigin: "Testland",
				Rarity: repository.RarityCommon, ForgerCharacterID: 3, Price: 10,
			},
			GuildID:   guildID,
			Quantity:  qty,
			ListingID: func() *int64 { id := int64(99); return &id }(),
		},
	}
	router := routerWithFake(fake)

	body := `{"name":"Iron Sword","land_of_origin":"Testland","forger_character_id":3,"rarity":"COMMON","price":10,"quantity":5,"guild_id":7}`
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastCreate.Name != "Iron Sword" || fake.lastCreate.Rarity != repository.RarityCommon {
		t.Errorf("service received CreateItemInput = %+v, want matching Name/Rarity", fake.lastCreate)
	}
	if fake.lastCreate.GuildID == nil || *fake.lastCreate.GuildID != guildID {
		t.Errorf("service received GuildID = %v, want %d", fake.lastCreate.GuildID, guildID)
	}
	if fake.lastCreate.Quantity == nil || *fake.lastCreate.Quantity != qty {
		t.Errorf("service received Quantity = %v, want %d", fake.lastCreate.Quantity, qty)
	}

	var resp createItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Item.ID != 42 || resp.GuildID != guildID || resp.Quantity != qty || resp.ListingID == nil || *resp.ListingID != 99 {
		t.Errorf("response = %+v, want ID=42 GuildID=%d Quantity=%d ListingID=99", resp, guildID, qty)
	}
}

func TestItemHandlers_Get_InvalidID_Returns400(t *testing.T) {
	fake := &fakeItemService{}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodGet, "/items/not-a-number", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestItemHandlers_Get_NotFound_Returns404(t *testing.T) {
	fake := &fakeItemService{getErr: service.ErrItemNotFound}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodGet, "/items/123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "ITEM_NOT_FOUND" {
		t.Errorf("error.code = %q, want ITEM_NOT_FOUND", env.Error.Code)
	}
	if fake.lastGet != 123 {
		t.Errorf("service received id = %d, want 123", fake.lastGet)
	}
}

func TestItemHandlers_Get_Success_MarksLegendaryAsAuctionOnly(t *testing.T) {
	fake := &fakeItemService{getView: service.ItemView{
		ID: 5, Name: "Doombringer", Rarity: repository.RarityLegendary, Price: 9000, IsLegendary: true,
	}}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodGet, "/items/5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp itemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.IsLegendary || !resp.AuctionOnly {
		t.Errorf("response = %+v, want IsLegendary=true AuctionOnly=true", resp)
	}
}

func TestItemHandlers_List_ReturnsItemsWrapped(t *testing.T) {
	fake := &fakeItemService{items: []service.ItemView{
		{ID: 1, Name: "A", Rarity: repository.RarityCommon},
		{ID: 2, Name: "B", Rarity: repository.RarityLegendary, IsLegendary: true},
	}}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodGet, "/items?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Items []itemResponse `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[1].IsLegendary != true {
		t.Errorf("response.Items = %+v, want 2 items with [1].IsLegendary=true", resp.Items)
	}
}

func TestItemHandlers_Purchase_MissingGuildHeader_Returns400(t *testing.T) {
	fake := &fakeItemService{}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/purchase", bytes.NewBufferString(`{"quantity":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "MISSING_GUILD_HEADER" {
		t.Errorf("error.code = %q, want MISSING_GUILD_HEADER", env.Error.Code)
	}
}

func TestItemHandlers_Purchase_MalformedGuildHeader_Returns400(t *testing.T) {
	fake := &fakeItemService{}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/purchase", bytes.NewBufferString(`{"quantity":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "not-a-number")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "INVALID_GUILD_HEADER" {
		t.Errorf("error.code = %q, want INVALID_GUILD_HEADER", env.Error.Code)
	}
}

func TestItemHandlers_Purchase_InvalidJSON_Returns400(t *testing.T) {
	fake := &fakeItemService{}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/purchase", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestItemHandlers_Purchase_Success_ReturnsPurchaseResult(t *testing.T) {
	fake := &fakeItemService{purchaseResult: service.PurchaseResult{
		ItemID: 1, Quantity: 2, UnitPrice: 10, TotalPrice: 20,
		SellerGuildID: 3, ListingID: 55, ListingStatus: repository.ListingActive,
	}}
	router := routerWithFake(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/purchase", bytes.NewBufferString(`{"quantity":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastPurchase.ItemID != 1 || fake.lastPurchase.BuyerGuildID != 9 || fake.lastPurchase.Quantity != 2 {
		t.Errorf("service received PurchaseInput = %+v, want ItemID=1 BuyerGuildID=9 Quantity=2", fake.lastPurchase)
	}

	var resp purchaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TotalPrice != 20 || resp.ListingStatus != repository.ListingActive {
		t.Errorf("response = %+v, want TotalPrice=20 ListingStatus=ACTIVE", resp)
	}
}

func TestItemHandlers_Purchase_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"item not found", service.ErrItemNotFound, http.StatusNotFound, "ITEM_NOT_FOUND"},
		{"listing not active", service.ErrListingNotActive, http.StatusConflict, "LISTING_NOT_ACTIVE"},
		{"insufficient quantity", service.ErrInsufficientQuantity, http.StatusConflict, "INSUFFICIENT_QUANTITY"},
		{"insufficient balance", service.ErrInsufficientBalance, http.StatusConflict, "INSUFFICIENT_BALANCE"},
		{"daily limit exceeded", service.ErrDailyLimitExceeded, http.StatusConflict, "DAILY_LIMIT_EXCEEDED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeItemService{purchaseErr: tc.err}
			router := routerWithFake(fake)

			req := httptest.NewRequest(http.MethodPost, "/items/1/purchase", bytes.NewBufferString(`{"quantity":1}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Guild-ID", "9")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			env := decodeErrorBody(t, rec)
			if env.Error.Code != tc.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}
