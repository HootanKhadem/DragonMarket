package oracle

import (
	"strconv"

	gocache "github.com/patrickmn/go-cache"
)

type Cache struct {
	c *gocache.Cache
}

func NewCache() *Cache {
	return &Cache{c: gocache.New(gocache.NoExpiration, gocache.NoExpiration)}
}

func (c *Cache) Get(itemID int64) (int, bool) {
	v, ok := c.c.Get(key(itemID))
	if !ok {
		return 0, false
	}
	price, ok := v.(int)
	return price, ok
}

func (c *Cache) Set(itemID int64, price int) {
	c.c.Set(key(itemID), price, gocache.NoExpiration)
}

func key(itemID int64) string {
	return strconv.FormatInt(itemID, 10)
}
