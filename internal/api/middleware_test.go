package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func routerWithGuildIDProbe() *gin.Engine {
	router := gin.New()
	router.POST("/probe", RequireGuildID(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"guild_id": guildIDFromContext(c)})
	})
	return router
}

func TestRequireGuildID_MissingHeader_Returns400AndAborts(t *testing.T) {
	router := routerWithGuildIDProbe()

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "MISSING_GUILD_HEADER" {
		t.Errorf("error.code = %q, want MISSING_GUILD_HEADER", env.Error.Code)
	}
}

func TestRequireGuildID_NonIntegerHeader_Returns400AndAborts(t *testing.T) {
	router := routerWithGuildIDProbe()

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-Guild-ID", "not-a-number")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	env := decodeErrorBody(t, rec)
	if env.Error.Code != "INVALID_GUILD_HEADER" {
		t.Errorf("error.code = %q, want INVALID_GUILD_HEADER", env.Error.Code)
	}
}

func TestRequireGuildID_ZeroOrNegativeHeader_Returns400(t *testing.T) {
	cases := []string{"0", "-1", "-42"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			router := routerWithGuildIDProbe()

			req := httptest.NewRequest(http.MethodPost, "/probe", nil)
			req.Header.Set("X-Guild-ID", raw)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
			env := decodeErrorBody(t, rec)
			if env.Error.Code != "INVALID_GUILD_HEADER" {
				t.Errorf("error.code = %q, want INVALID_GUILD_HEADER", env.Error.Code)
			}
		})
	}
}

func TestRequireGuildID_ValidHeader_SetsContextAndCallsNext(t *testing.T) {
	router := routerWithGuildIDProbe()

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-Guild-ID", "42")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		GuildID int64 `json:"guild_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.GuildID != 42 {
		t.Errorf("guild_id = %d, want 42", resp.GuildID)
	}
}

func TestGuildIDFromContext_NotSet_ReturnsZero(t *testing.T) {
	router := gin.New()
	router.GET("/no-middleware", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"guild_id": guildIDFromContext(c)})
	})

	req := httptest.NewRequest(http.MethodGet, "/no-middleware", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		GuildID int64 `json:"guild_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.GuildID != 0 {
		t.Errorf("guild_id = %d, want 0 (accessor called without middleware wired)", resp.GuildID)
	}
}
