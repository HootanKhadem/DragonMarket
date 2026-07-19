package oracle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"DragonMarket/internal/oracle"
)

func TestMockPriceOracleService_DefaultReturnsConfiguredPrice(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 4200})

	price, err := m.GetPrice(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPrice() error = %v, want nil", err)
	}
	if price != 4200 {
		t.Errorf("GetPrice() = %d, want 4200", price)
	}
}

func TestMockPriceOracleService_PerItemOverridesDefault(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 100})
	m.SetForItem(7, oracle.MockResponse{Price: 999})

	price, err := m.GetPrice(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetPrice(7) error = %v, want nil", err)
	}
	if price != 999 {
		t.Errorf("GetPrice(7) = %d, want 999 (per-item override)", price)
	}

	price, err = m.GetPrice(context.Background(), 8)
	if err != nil {
		t.Fatalf("GetPrice(8) error = %v, want nil", err)
	}
	if price != 100 {
		t.Errorf("GetPrice(8) = %d, want 100 (default)", price)
	}
}

func TestMockPriceOracleService_ConfiguredError(t *testing.T) {
	wantErr := errors.New("oracle unavailable")
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Err: wantErr})

	_, err := m.GetPrice(context.Background(), 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetPrice() error = %v, want %v", err, wantErr)
	}
}

func TestMockPriceOracleService_ZeroAndNegativePricesPassThrough(t *testing.T) {
	m := oracle.NewMockPriceOracleService()

	m.SetForItem(1, oracle.MockResponse{Price: 0})
	price, err := m.GetPrice(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPrice(1) error = %v, want nil", err)
	}
	if price != 0 {
		t.Errorf("GetPrice(1) = %d, want 0", price)
	}

	m.SetForItem(2, oracle.MockResponse{Price: -50})
	price, err = m.GetPrice(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetPrice(2) error = %v, want nil", err)
	}
	if price != -50 {
		t.Errorf("GetPrice(2) = %d, want -50", price)
	}
}

func TestMockPriceOracleService_DelayRespectsContextTimeout(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 10, Delay: 100 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := m.GetPrice(ctx, 1)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetPrice() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("GetPrice() took %v, want it to return promptly after ctx deadline", elapsed)
	}
}

func TestMockPriceOracleService_JitterStaysWithinBounds(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 1000, Jitter: 50})

	for range 200 {
		price, err := m.GetPrice(context.Background(), 1)
		if err != nil {
			t.Fatalf("GetPrice() error = %v, want nil", err)
		}
		if price < 950 || price > 1050 {
			t.Fatalf("GetPrice() = %d, want within [950, 1050]", price)
		}
	}
}

func TestMockPriceOracleService_ZeroJitterIsDeterministic(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 1000})

	for range 20 {
		price, err := m.GetPrice(context.Background(), 1)
		if err != nil {
			t.Fatalf("GetPrice() error = %v, want nil", err)
		}
		if price != 1000 {
			t.Fatalf("GetPrice() = %d, want exactly 1000 with no jitter configured", price)
		}
	}
}

func TestMockPriceOracleService_DelayShorterThanTimeoutSucceeds(t *testing.T) {
	m := oracle.NewMockPriceOracleService()
	m.SetDefault(oracle.MockResponse{Price: 77, Delay: 5 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	price, err := m.GetPrice(ctx, 1)
	if err != nil {
		t.Fatalf("GetPrice() error = %v, want nil", err)
	}
	if price != 77 {
		t.Errorf("GetPrice() = %d, want 77", price)
	}
}
