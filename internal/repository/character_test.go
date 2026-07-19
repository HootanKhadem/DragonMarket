package repository

import (
	"context"
	"errors"
	"testing"
)

func TestCharacterRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewCharacterRepository()

	createdCharacter, err := repo.Create(ctx, tx, Character{
		Name:         "Test Hero",
		LandOfOrigin: "Testland",
		Stats:        new("Str: +1"),
		GuildID:      nil,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdCharacter.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}

	fetchedCharacter, err := repo.GetByID(ctx, tx, createdCharacter.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedCharacter.Name != "Test Hero" || fetchedCharacter.LandOfOrigin != "Testland" {
		t.Errorf("GetByID() = %+v, want matching Name/LandOfOrigin", fetchedCharacter)
	}
	if fetchedCharacter.Stats == nil || *fetchedCharacter.Stats != "Str: +1" {
		t.Errorf("GetByID().Stats = %v, want \"Str: +1\"", fetchedCharacter.Stats)
	}
	if fetchedCharacter.GuildID != nil {
		t.Errorf("GetByID().GuildID = %v, want nil", fetchedCharacter.GuildID)
	}
}

func TestCharacterRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewCharacterRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestCharacterRepository_List(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewCharacterRepository()

	before, err := repo.List(ctx, tx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if _, err := repo.Create(ctx, tx, Character{Name: "Lister One", LandOfOrigin: "X"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := repo.Create(ctx, tx, Character{Name: "Lister Two", LandOfOrigin: "Y"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	after, err := repo.List(ctx, tx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(after) != len(before)+2 {
		t.Errorf("List() len = %d, want %d (before %d + 2 created)", len(after), len(before)+2, len(before))
	}
}
