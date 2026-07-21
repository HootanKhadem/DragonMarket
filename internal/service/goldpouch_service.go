package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"DragonMarket/internal/repository"
)

var (
	ErrInvalidAmount          = errors.New("service: amount must be positive")
	ErrInsufficientBalance    = errors.New("service: insufficient usable balance")
	ErrInsufficientReserved   = errors.New("service: insufficient reserved balance")
	ErrDailyLimitExceeded     = errors.New("service: daily spending limit exceeded")
	ErrInvalidTransactionType = errors.New("service: invalid transaction type for settle")
)

type GoldPouchService struct {
	pouches repository.GoldPouchRepository
	logs    repository.TransactionLogRepository
}

func NewGoldPouchService(pouches repository.GoldPouchRepository, logs repository.TransactionLogRepository) *GoldPouchService {
	return &GoldPouchService{pouches: pouches, logs: logs}
}

func (s *GoldPouchService) Reserve(ctx context.Context, tx pgx.Tx, guildID int64, amount int, reference *string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	gp, err := s.pouches.GetByGuildIDForUpdate(ctx, tx, guildID)
	if err != nil {
		return fmt.Errorf("service: reserve: %w", err)
	}
	if gp.UsableBalance < amount {
		return ErrInsufficientBalance
	}

	today := utcMidnight(time.Now())
	if !sameUTCDate(gp.LastResetDate, today) {
		gp.SpentToday = 0
		gp.LastResetDate = today
	}
	if gp.SpentToday+amount > gp.DailySpendingLimit {
		return ErrDailyLimitExceeded
	}

	gp.ReservedBalance += amount
	if err := s.pouches.Update(ctx, tx, gp); err != nil {
		return fmt.Errorf("service: reserve: update pouch: %w", err)
	}

	if err := s.writeLog(ctx, tx, guildID, repository.TxReserve, amount, reference); err != nil {
		return fmt.Errorf("service: reserve: %w", err)
	}
	return nil
}

func (s *GoldPouchService) Release(ctx context.Context, tx pgx.Tx, guildID int64, amount int, reference *string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	gp, err := s.pouches.GetByGuildIDForUpdate(ctx, tx, guildID)
	if err != nil {
		return fmt.Errorf("service: release: %w", err)
	}
	if gp.ReservedBalance < amount {
		return ErrInsufficientReserved
	}

	gp.ReservedBalance -= amount
	if err := s.pouches.Update(ctx, tx, gp); err != nil {
		return fmt.Errorf("service: release: update pouch: %w", err)
	}

	if err := s.writeLog(ctx, tx, guildID, repository.TxRelease, amount, reference); err != nil {
		return fmt.Errorf("service: release: %w", err)
	}
	return nil
}

func (s *GoldPouchService) Settle(ctx context.Context, tx pgx.Tx, guildID int64, amount int, txType repository.TransactionType, reference *string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if txType != repository.TxPurchase && txType != repository.TxAuctionWin {
		return ErrInvalidTransactionType
	}

	gp, err := s.pouches.GetByGuildIDForUpdate(ctx, tx, guildID)
	if err != nil {
		return fmt.Errorf("service: settle: %w", err)
	}
	if gp.ReservedBalance < amount {
		return ErrInsufficientReserved
	}

	today := utcMidnight(time.Now())
	if !sameUTCDate(gp.LastResetDate, today) {
		gp.SpentToday = 0
		gp.LastResetDate = today
	}
	if gp.SpentToday+amount > gp.DailySpendingLimit {
		return ErrDailyLimitExceeded
	}

	gp.TotalBalance -= amount
	gp.ReservedBalance -= amount
	gp.SpentToday += amount
	if err := s.pouches.Update(ctx, tx, gp); err != nil {
		return fmt.Errorf("service: settle: update pouch: %w", err)
	}

	if err := s.writeLog(ctx, tx, guildID, txType, amount, reference); err != nil {
		return fmt.Errorf("service: settle: %w", err)
	}
	return nil
}

func (s *GoldPouchService) Credit(ctx context.Context, tx pgx.Tx, guildID int64, amount int, reference *string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	gp, err := s.pouches.GetByGuildIDForUpdate(ctx, tx, guildID)
	if err != nil {
		return fmt.Errorf("service: credit: %w", err)
	}

	gp.TotalBalance += amount
	if err := s.pouches.Update(ctx, tx, gp); err != nil {
		return fmt.Errorf("service: credit: update pouch: %w", err)
	}

	if err := s.writeLog(ctx, tx, guildID, repository.TxCredit, amount, reference); err != nil {
		return fmt.Errorf("service: credit: %w", err)
	}
	return nil
}

func (s *GoldPouchService) writeLog(ctx context.Context, tx pgx.Tx, guildID int64, txType repository.TransactionType, amount int, reference *string) error {
	_, err := s.logs.Create(ctx, tx, repository.TransactionLog{
		GuildID:   guildID,
		Type:      txType,
		Amount:    amount,
		Reference: reference,
	})
	return err
}

func utcMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func sameUTCDate(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
