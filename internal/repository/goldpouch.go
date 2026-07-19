package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type GoldPouch struct {
	ID                 int64
	GuildID            int64
	TotalBalance       int
	ReservedBalance    int
	UsableBalance      int // read-only, generated
	DailySpendingLimit int
	SpentToday         int
	LastResetDate      time.Time
}

type GoldPouchRepository interface {
	Create(ctx context.Context, db DBTX, gp GoldPouch) (GoldPouch, error)
	GetByGuildID(ctx context.Context, db DBTX, guildID int64) (GoldPouch, error)
	GetByGuildIDForUpdate(ctx context.Context, tx pgx.Tx, guildID int64) (GoldPouch, error)
	Update(ctx context.Context, db DBTX, gp GoldPouch) error
}

type goldPouchRepository struct{}

// NewGoldPouchRepository returns a pgx-backed GoldPouchRepository.
func NewGoldPouchRepository() GoldPouchRepository {
	return &goldPouchRepository{}
}

const goldPouchColumns = `id, guild_id, total_balance, reserved_balance, usable_balance,
	daily_spending_limit, spent_today, last_reset_date`

func (r *goldPouchRepository) Create(ctx context.Context, db DBTX, gp GoldPouch) (GoldPouch, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO gold_pouches
		   (guild_id, total_balance, reserved_balance, daily_spending_limit, spent_today, last_reset_date)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, usable_balance`,
		gp.GuildID, gp.TotalBalance, gp.ReservedBalance, gp.DailySpendingLimit, gp.SpentToday, gp.LastResetDate,
	).Scan(&gp.ID, &gp.UsableBalance)
	if err != nil {
		return GoldPouch{}, err
	}
	return gp, nil
}

func (r *goldPouchRepository) GetByGuildID(ctx context.Context, db DBTX, guildID int64) (GoldPouch, error) {
	return r.getBy(ctx, db, guildID, false)
}

func (r *goldPouchRepository) GetByGuildIDForUpdate(ctx context.Context, tx pgx.Tx, guildID int64) (GoldPouch, error) {
	return r.getBy(ctx, tx, guildID, true)
}

func (r *goldPouchRepository) getBy(ctx context.Context, db DBTX, guildID int64, forUpdate bool) (GoldPouch, error) {
	query := `SELECT ` + goldPouchColumns + ` FROM gold_pouches WHERE guild_id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}
	var gp GoldPouch
	err := db.QueryRow(ctx, query, guildID).Scan(
		&gp.ID, &gp.GuildID, &gp.TotalBalance, &gp.ReservedBalance, &gp.UsableBalance,
		&gp.DailySpendingLimit, &gp.SpentToday, &gp.LastResetDate,
	)
	if err != nil {
		return GoldPouch{}, scanErr(err)
	}
	return gp, nil
}

func (r *goldPouchRepository) Update(ctx context.Context, db DBTX, gp GoldPouch) error {
	_, err := db.Exec(ctx,
		`UPDATE gold_pouches
		 SET total_balance = $2, reserved_balance = $3, daily_spending_limit = $4,
		     spent_today = $5, last_reset_date = $6
		 WHERE guild_id = $1`,
		gp.GuildID, gp.TotalBalance, gp.ReservedBalance, gp.DailySpendingLimit, gp.SpentToday, gp.LastResetDate,
	)
	return err
}
