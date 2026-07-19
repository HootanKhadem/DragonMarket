package repository

import (
	"context"
	"errors"
	"testing"
)

func TestTransactionLogRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "TxLog Test Guild 1")

	repo := NewTransactionLogRepository()
	ref := "item:42"
	createdTransactionLog, err := repo.Create(ctx, tx, TransactionLog{
		GuildID:   guild.ID,
		Type:      TxPurchase,
		Amount:    150,
		Reference: &ref,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdTransactionLog.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}
	if createdTransactionLog.CreatedAt.IsZero() {
		t.Errorf("Create().CreatedAt is zero, want default now()")
	}

	fetchedTransactionLog, err := repo.GetByID(ctx, tx, createdTransactionLog.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedTransactionLog.Type != TxPurchase || fetchedTransactionLog.Amount != 150 || fetchedTransactionLog.Reference == nil || *fetchedTransactionLog.Reference != "item:42" {
		t.Errorf("GetByID() = %+v, want Type=PURCHASE Amount=150 Reference=item:42", fetchedTransactionLog)
	}
}

func TestTransactionLogRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewTransactionLogRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestTransactionLogRepository_ListByGuildID(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "TxLog Test Guild 2")
	repo := NewTransactionLogRepository()

	before, err := repo.ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}

	if _, err := repo.Create(ctx, tx, TransactionLog{GuildID: guild.ID, Type: TxCredit, Amount: 100}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := repo.Create(ctx, tx, TransactionLog{GuildID: guild.ID, Type: TxReserve, Amount: 50}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	after, err := repo.ListByGuildID(ctx, tx, guild.ID)
	if err != nil {
		t.Fatalf("ListByGuildID() error = %v", err)
	}
	if len(after) != len(before)+2 {
		t.Errorf("ListByGuildID() len = %d, want %d", len(after), len(before)+2)
	}
}

func TestTransactionLogRepository_Create_NilReference(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	guild := createTestGuild(t, ctx, tx, "TxLog Test Guild 3")
	repo := NewTransactionLogRepository()

	createdTransactionLog, err := repo.Create(ctx, tx, TransactionLog{GuildID: guild.ID, Type: TxAuctionWin, Amount: 9000, Reference: nil})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	fetchedTransactionLog, err := repo.GetByID(ctx, tx, createdTransactionLog.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedTransactionLog.Reference != nil {
		t.Errorf("GetByID().Reference = %v, want nil", fetchedTransactionLog.Reference)
	}
}
