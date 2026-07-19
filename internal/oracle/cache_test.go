package oracle_test

import (
	"testing"

	"DragonMarket/internal/oracle"
)

func TestCache_GetOnEmptyCacheMisses(t *testing.T) {
	c := oracle.NewCache()

	_, ok := c.Get(1)
	if ok {
		t.Fatalf("Get() on empty cache: ok = true, want false")
	}
}

func TestCache_SetThenGetReturnsStoredPrice(t *testing.T) {
	c := oracle.NewCache()

	c.Set(1, 12345)

	price, ok := c.Get(1)
	if !ok {
		t.Fatalf("Get() ok = false, want true after Set()")
	}
	if price != 12345 {
		t.Errorf("Get() = %d, want 12345", price)
	}
}

func TestCache_SetOverwritesPreviousValue(t *testing.T) {
	c := oracle.NewCache()

	c.Set(1, 100)
	c.Set(1, 200)

	price, ok := c.Get(1)
	if !ok || price != 200 {
		t.Errorf("Get() = (%d, %v), want (200, true)", price, ok)
	}
}

func TestCache_DifferentItemsAreIndependent(t *testing.T) {
	c := oracle.NewCache()

	c.Set(1, 100)
	c.Set(2, 200)

	p1, ok1 := c.Get(1)
	p2, ok2 := c.Get(2)
	if !ok1 || p1 != 100 {
		t.Errorf("Get(1) = (%d, %v), want (100, true)", p1, ok1)
	}
	if !ok2 || p2 != 200 {
		t.Errorf("Get(2) = (%d, %v), want (200, true)", p2, ok2)
	}
}
