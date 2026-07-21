package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"DragonMarket/internal/repository"
)

type WalletView struct {
	GuildID            int64
	TotalBalance       int
	ReservedBalance    int
	UsableBalance      int
	DailySpendingLimit int
	SpentToday         int
}

type WalletService struct {
	db      repository.DBTX
	guilds  repository.GuildRepository
	pouches repository.GoldPouchRepository
}

func NewWalletService(db repository.DBTX, guilds repository.GuildRepository, pouches repository.GoldPouchRepository) *WalletService {
	return &WalletService{db: db, guilds: guilds, pouches: pouches}
}

func (s *WalletService) GetWallet(ctx context.Context, guildID int64) (WalletView, error) {
	if _, err := s.guilds.GetByID(ctx, s.db, guildID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return WalletView{}, ErrGuildNotFound
		}
		return WalletView{}, fmt.Errorf("service: get wallet: get guild: %w", err)
	}

	gp, err := s.pouches.GetByGuildID(ctx, s.db, guildID)
	if err != nil {
		return WalletView{}, fmt.Errorf("service: get wallet: get pouch: %w", err)
	}

	spentToday := gp.SpentToday
	if !sameUTCDate(gp.LastResetDate, utcMidnight(time.Now())) {
		spentToday = 0
	}

	return WalletView{
		GuildID:            gp.GuildID,
		TotalBalance:       gp.TotalBalance,
		ReservedBalance:    gp.ReservedBalance,
		UsableBalance:      gp.UsableBalance,
		DailySpendingLimit: gp.DailySpendingLimit,
		SpentToday:         spentToday,
	}, nil
}
