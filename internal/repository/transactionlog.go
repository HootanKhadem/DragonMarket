package repository

import (
	"context"
	"time"
)

type TransactionType string

const (
	TxPurchase   TransactionType = "PURCHASE"
	TxReserve    TransactionType = "RESERVE"
	TxRelease    TransactionType = "RELEASE"
	TxCredit     TransactionType = "CREDIT"
	TxAuctionWin TransactionType = "AUCTION_WIN"
)

type TransactionLog struct {
	ID        int64
	GuildID   int64
	Type      TransactionType
	Amount    int
	Reference *string // nullable
	CreatedAt time.Time
}

type TransactionLogRepository interface {
	Create(ctx context.Context, db DBTX, tl TransactionLog) (TransactionLog, error)
	GetByID(ctx context.Context, db DBTX, id int64) (TransactionLog, error)
	ListByGuildID(ctx context.Context, db DBTX, guildID int64) ([]TransactionLog, error)
}

type transactionLogRepository struct{}

func NewTransactionLogRepository() TransactionLogRepository {
	return &transactionLogRepository{}
}

const transactionLogColumns = `id, guild_id, type, amount, reference, created_at`

func (r *transactionLogRepository) Create(ctx context.Context, db DBTX, tl TransactionLog) (TransactionLog, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO transaction_logs (guild_id, type, amount, reference)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		tl.GuildID, tl.Type, tl.Amount, tl.Reference,
	).Scan(&tl.ID, &tl.CreatedAt)
	if err != nil {
		return TransactionLog{}, err
	}
	return tl, nil
}

func (r *transactionLogRepository) GetByID(ctx context.Context, db DBTX, id int64) (TransactionLog, error) {
	var tl TransactionLog
	err := db.QueryRow(ctx, `SELECT `+transactionLogColumns+` FROM transaction_logs WHERE id = $1`, id).
		Scan(&tl.ID, &tl.GuildID, &tl.Type, &tl.Amount, &tl.Reference, &tl.CreatedAt)
	if err != nil {
		return TransactionLog{}, scanErr(err)
	}
	return tl, nil
}

func (r *transactionLogRepository) ListByGuildID(ctx context.Context, db DBTX, guildID int64) ([]TransactionLog, error) {
	rows, err := db.Query(ctx,
		`SELECT `+transactionLogColumns+` FROM transaction_logs WHERE guild_id = $1 ORDER BY id`,
		guildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TransactionLog
	for rows.Next() {
		var tl TransactionLog
		if err := rows.Scan(&tl.ID, &tl.GuildID, &tl.Type, &tl.Amount, &tl.Reference, &tl.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, tl)
	}
	return out, rows.Err()
}
