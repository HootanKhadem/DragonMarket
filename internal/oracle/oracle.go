package oracle

import "context"

type PriceOracleService interface {
	GetPrice(ctx context.Context, itemID int64) (int, error)
}
