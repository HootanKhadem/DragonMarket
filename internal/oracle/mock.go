package oracle

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"
)

type MockResponse struct {
	Price  int
	Jitter int
	Err    error
	Delay  time.Duration
}

type MockPriceOracleService struct {
	mu      sync.Mutex
	def     MockResponse
	perItem map[int64]MockResponse
}

func NewMockPriceOracleService() *MockPriceOracleService {
	return &MockPriceOracleService{
		perItem: make(map[int64]MockResponse),
	}
}

func (m *MockPriceOracleService) SetDefault(resp MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.def = resp
}

func (m *MockPriceOracleService) SetForItem(itemID int64, resp MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.perItem[itemID] = resp
}

func (m *MockPriceOracleService) GetPrice(ctx context.Context, itemID int64) (int, error) {
	resp := m.responseFor(itemID)

	if resp.Delay > 0 {
		timer := time.NewTimer(resp.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	if resp.Err != nil {
		return 0, resp.Err
	}
	price := resp.Price
	if resp.Jitter > 0 {
		price += rand.IntN(2*resp.Jitter+1) - resp.Jitter
	}
	return price, nil
}

func (m *MockPriceOracleService) responseFor(itemID int64) MockResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	if resp, ok := m.perItem[itemID]; ok {
		return resp
	}
	return m.def
}
