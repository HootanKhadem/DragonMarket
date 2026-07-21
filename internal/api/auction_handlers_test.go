package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/repository"
	"DragonMarket/internal/service"
)

type fakeAuctionService struct {
	createResult service.AuctionView
	createErr    error
	lastCreate   service.CreateAuctionInput

	auctions []service.AuctionView
	listErr  error

	getView service.AuctionView
	getErr  error
	lastGet int64

	bidResult service.BidView
	bidErr    error
	lastBid   service.PlaceBidInput

	cancelErr  error
	lastCancel service.CancelBidInput
}

func (f *fakeAuctionService) CreateAuction(ctx context.Context, in service.CreateAuctionInput) (service.AuctionView, error) {
	f.lastCreate = in
	return f.createResult, f.createErr
}

func (f *fakeAuctionService) GetAuction(ctx context.Context, id int64) (service.AuctionView, error) {
	f.lastGet = id
	return f.getView, f.getErr
}

func (f *fakeAuctionService) ListActiveAuctions(ctx context.Context, limit, offset int) ([]service.AuctionView, error) {
	return f.auctions, f.listErr
}

func (f *fakeAuctionService) PlaceBid(ctx context.Context, in service.PlaceBidInput) (service.BidView, error) {
	f.lastBid = in
	return f.bidResult, f.bidErr
}

func (f *fakeAuctionService) CancelBid(ctx context.Context, in service.CancelBidInput) error {
	f.lastCancel = in
	return f.cancelErr
}

func routerWithFakeAuctions(fake *fakeAuctionService) *gin.Engine {
	h := NewAuctionHandlers(fake)
	router := gin.New()
	router.POST("/auctions", h.Create)
	router.GET("/auctions", h.List)
	router.GET("/auctions/:id", h.Get)
	router.POST("/items/:id/bid", h.PlaceBid)
	router.DELETE("/items/:id/bid/:bid_id", h.CancelBid)
	return router
}

func TestAuctionHandlers_Create_MissingGuildHeader_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/auctions", bytes.NewBufferString(`{"item_id":1,"duration_seconds":3600}`))
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

func TestAuctionHandlers_Create_InvalidJSON_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/auctions", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", env.Error.Code)
	}
}

func TestAuctionHandlers_Create_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid duration", service.ErrInvalidDuration, http.StatusBadRequest, "INVALID_DURATION"},
		{"item not found", service.ErrItemNotFound, http.StatusNotFound, "ITEM_NOT_FOUND"},
		{"not item owner", service.ErrNotItemOwner, http.StatusConflict, "NOT_ITEM_OWNER"},
		{"not legendary", service.ErrItemNotLegendary, http.StatusBadRequest, "ITEM_NOT_LEGENDARY"},
		{"auction already exists", service.ErrAuctionAlreadyExists, http.StatusConflict, "AUCTION_ALREADY_EXISTS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuctionService{createErr: tc.err}
			router := routerWithFakeAuctions(fake)

			req := httptest.NewRequest(http.MethodPost, "/auctions", bytes.NewBufferString(`{"item_id":1,"duration_seconds":3600}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Guild-ID", "7")
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

func TestAuctionHandlers_Create_Success_PassesOwnerGuildFromHeaderAndReturns201(t *testing.T) {
	fake := &fakeAuctionService{createResult: service.AuctionView{
		ID: 5, ItemID: 1, OwnerGuildID: 7, Status: repository.AuctionActive,
		StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), BasePrice: 9000,
	}}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/auctions", bytes.NewBufferString(`{"item_id":1,"duration_seconds":3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "7")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastCreate.ItemID != 1 || fake.lastCreate.OwnerGuildID != 7 || fake.lastCreate.DurationSeconds != 3600 {
		t.Errorf("service received CreateAuctionInput = %+v, want ItemID=1 OwnerGuildID=7 DurationSeconds=3600", fake.lastCreate)
	}

	var resp auctionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 5 || resp.BasePrice != 9000 || resp.Status != repository.AuctionActive {
		t.Errorf("response = %+v, want ID=5 BasePrice=9000 Status=ACTIVE", resp)
	}
}

func TestAuctionHandlers_List_ReturnsAuctionsWrapped(t *testing.T) {
	fake := &fakeAuctionService{auctions: []service.AuctionView{
		{ID: 1, ItemID: 10, Status: repository.AuctionActive, BasePrice: 100},
		{ID: 2, ItemID: 11, Status: repository.AuctionActive, BasePrice: 200},
	}}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodGet, "/auctions?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Auctions []auctionResponse `json:"auctions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Auctions) != 2 || resp.Auctions[1].BasePrice != 200 {
		t.Errorf("response.Auctions = %+v, want 2 auctions with [1].BasePrice=200", resp.Auctions)
	}
}

func TestAuctionHandlers_Get_InvalidID_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodGet, "/auctions/not-a-number", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuctionHandlers_Get_NotFound_Returns404(t *testing.T) {
	fake := &fakeAuctionService{getErr: service.ErrAuctionNotFound}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodGet, "/auctions/42", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "AUCTION_NOT_FOUND" {
		t.Errorf("error.code = %q, want AUCTION_NOT_FOUND", env.Error.Code)
	}
	if fake.lastGet != 42 {
		t.Errorf("service received id = %d, want 42", fake.lastGet)
	}
}

func TestAuctionHandlers_Get_Success_ReturnsAuction(t *testing.T) {
	fake := &fakeAuctionService{getView: service.AuctionView{
		ID: 9, ItemID: 3, OwnerGuildID: 2, Status: repository.AuctionActive, BasePrice: 5000,
	}}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodGet, "/auctions/9", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp auctionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 9 || resp.BasePrice != 5000 {
		t.Errorf("response = %+v, want ID=9 BasePrice=5000", resp)
	}
}

func TestAuctionHandlers_PlaceBid_MissingGuildHeader_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/bid", bytes.NewBufferString(`{"amount":100}`))
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

func TestAuctionHandlers_PlaceBid_InvalidJSON_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/bid", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuctionHandlers_PlaceBid_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"auction not found", service.ErrAuctionNotFound, http.StatusNotFound, "AUCTION_NOT_FOUND"},
		{"auction not active", service.ErrAuctionNotActive, http.StatusConflict, "AUCTION_NOT_ACTIVE"},
		{"auction expired", service.ErrAuctionExpired, http.StatusConflict, "AUCTION_EXPIRED"},
		{"self bid", service.ErrSelfBidNotAllowed, http.StatusConflict, "SELF_BID_NOT_ALLOWED"},
		{"bid too low", service.ErrBidTooLow, http.StatusConflict, "BID_TOO_LOW"},
		{"insufficient balance", service.ErrInsufficientBalance, http.StatusConflict, "INSUFFICIENT_BALANCE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuctionService{bidErr: tc.err}
			router := routerWithFakeAuctions(fake)

			req := httptest.NewRequest(http.MethodPost, "/items/1/bid", bytes.NewBufferString(`{"amount":100}`))
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

func TestAuctionHandlers_PlaceBid_Success_PassesItemIDGuildIDAmountAndReturns201(t *testing.T) {
	fake := &fakeAuctionService{bidResult: service.BidView{
		ID: 3, AuctionID: 1, GuildID: 9, Amount: 150, Status: repository.BidActive,
	}}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodPost, "/items/1/bid", bytes.NewBufferString(`{"amount":150}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastBid.ItemID != 1 || fake.lastBid.GuildID != 9 || fake.lastBid.Amount != 150 {
		t.Errorf("service received PlaceBidInput = %+v, want ItemID=1 GuildID=9 Amount=150", fake.lastBid)
	}

	var resp bidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 3 || resp.Amount != 150 || resp.Status != repository.BidActive {
		t.Errorf("response = %+v, want ID=3 Amount=150 Status=ACTIVE", resp)
	}
}

func TestAuctionHandlers_CancelBid_MissingGuildHeader_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodDelete, "/items/1/bid/5", nil)
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

func TestAuctionHandlers_CancelBid_InvalidBidID_Returns400(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodDelete, "/items/1/bid/not-a-number", nil)
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuctionHandlers_CancelBid_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"bid not found", service.ErrBidNotFound, http.StatusNotFound, "BID_NOT_FOUND"},
		{"not bid owner", service.ErrNotBidOwner, http.StatusConflict, "NOT_BID_OWNER"},
		{"auction not active", service.ErrAuctionNotActive, http.StatusConflict, "AUCTION_NOT_ACTIVE"},
		{"bid is highest", service.ErrBidIsHighest, http.StatusConflict, "BID_IS_HIGHEST"},
		{"bid not active", service.ErrBidNotActive, http.StatusConflict, "BID_NOT_ACTIVE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuctionService{cancelErr: tc.err}
			router := routerWithFakeAuctions(fake)

			req := httptest.NewRequest(http.MethodDelete, "/items/1/bid/5", nil)
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

func TestAuctionHandlers_CancelBid_Success_Returns204AndPassesFields(t *testing.T) {
	fake := &fakeAuctionService{}
	router := routerWithFakeAuctions(fake)

	req := httptest.NewRequest(http.MethodDelete, "/items/1/bid/5", nil)
	req.Header.Set("X-Guild-ID", "9")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastCancel.ItemID != 1 || fake.lastCancel.BidID != 5 || fake.lastCancel.GuildID != 9 {
		t.Errorf("service received CancelBidInput = %+v, want ItemID=1 BidID=5 GuildID=9", fake.lastCancel)
	}
}
