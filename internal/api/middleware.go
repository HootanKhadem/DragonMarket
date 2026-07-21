package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const guildIDContextKey = "guildID"

func RequireGuildID() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("X-Guild-ID")
		if raw == "" {
			writeError(c, http.StatusBadRequest, "MISSING_GUILD_HEADER", "X-Guild-ID header is required")
			c.Abort()
			return
		}

		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_GUILD_HEADER", "X-Guild-ID header must be a positive integer")
			c.Abort()
			return
		}

		c.Set(guildIDContextKey, id)
		c.Next()
	}
}

func validateItemIDParam() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := parseItemID(c); err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_ITEM_ID", err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}

func validateBidIDParam() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := parseInt64Param(c, "bid_id"); err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_BID_ID", err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}

func guildIDFromContext(c *gin.Context) int64 {
	v, ok := c.Get(guildIDContextKey)
	if !ok {
		return 0
	}
	id, ok := v.(int64)
	if !ok {
		return 0
	}
	return id
}
