package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"DragonMarket/internal/service"
)

type fakeWalletService struct {
	view    service.WalletView
	err     error
	lastGet int64
}

func (f *fakeWalletService) GetWallet(ctx context.Context, guildID int64) (service.WalletView, error) {
	f.lastGet = guildID
	return f.view, f.err
}

func routerWithFakeWallet(fake *fakeWalletService) *gin.Engine {
	h := NewWalletHandlers(fake)
	router := gin.New()
	router.GET("/guilds/:id/wallet", h.Get)
	return router
}

func TestWalletHandlers_Get_InvalidGuildID_Returns400(t *testing.T) {
	fake := &fakeWalletService{}
	router := routerWithFakeWallet(fake)

	req := httptest.NewRequest(http.MethodGet, "/guilds/not-a-number/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestWalletHandlers_Get_GuildNotFound_Returns404(t *testing.T) {
	fake := &fakeWalletService{err: service.ErrGuildNotFound}
	router := routerWithFakeWallet(fake)

	req := httptest.NewRequest(http.MethodGet, "/guilds/42/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "GUILD_NOT_FOUND" {
		t.Errorf("error.code = %q, want GUILD_NOT_FOUND", env.Error.Code)
	}
	if fake.lastGet != 42 {
		t.Errorf("service received guildID = %d, want 42", fake.lastGet)
	}
}

func TestWalletHandlers_Get_Success_ReturnsWalletFields(t *testing.T) {
	fake := &fakeWalletService{view: service.WalletView{
		GuildID:            7,
		TotalBalance:       1000,
		ReservedBalance:    200,
		UsableBalance:      800,
		DailySpendingLimit: 500,
		SpentToday:         150,
	}}
	router := routerWithFakeWallet(fake)

	req := httptest.NewRequest(http.MethodGet, "/guilds/7/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.lastGet != 7 {
		t.Errorf("service received guildID = %d, want 7", fake.lastGet)
	}

	var resp walletResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := walletResponse{
		TotalBalance:       1000,
		ReservedBalance:    200,
		UsableBalance:      800,
		DailySpendingLimit: 500,
		SpentToday:         150,
	}
	if resp != want {
		t.Errorf("response = %+v, want %+v", resp, want)
	}
}

func TestWalletHandlers_Get_Success_StaleResetPassesThroughAsZero(t *testing.T) {
	fake := &fakeWalletService{view: service.WalletView{
		GuildID:            3,
		TotalBalance:       500,
		ReservedBalance:    0,
		UsableBalance:      500,
		DailySpendingLimit: 100,
		SpentToday:         0,
	}}
	router := routerWithFakeWallet(fake)

	req := httptest.NewRequest(http.MethodGet, "/guilds/3/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp walletResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SpentToday != 0 {
		t.Errorf("response.SpentToday = %d, want 0", resp.SpentToday)
	}
}
