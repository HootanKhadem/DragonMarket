package repository

import (
	"context"
	"errors"
	"testing"
)

// createTestCharacter is a small test fixture helper: guilds/items require
// a character (leader/forger), so most guild/item/etc. tests need a
// character row first.
func createTestCharacter(t *testing.T, ctx context.Context, tx DBTX, name string) Character {
	t.Helper()
	c, err := NewCharacterRepository().Create(ctx, tx, Character{
		Name:         name,
		LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create character %q: %v", name, err)
	}
	return c
}

func TestGuildRepository_CreateThenGet(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	leader := createTestCharacter(t, ctx, tx, "Guild Leader One")

	repo := NewGuildRepository()
	createdGuild, err := repo.Create(ctx, tx, Guild{
		Name:              "Test Guild Alpha",
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdGuild.ID == 0 {
		t.Fatalf("Create() returned zero ID")
	}

	fetchedGuild, err := repo.GetByID(ctx, tx, createdGuild.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetchedGuild.Name != "Test Guild Alpha" || fetchedGuild.LeaderCharacterID != leader.ID {
		t.Errorf("GetByID() = %+v, want Name=Test Guild Alpha LeaderCharacterID=%d", fetchedGuild, leader.ID)
	}
}

func TestGuildRepository_GetByID_NotFound(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewGuildRepository()

	_, err := repo.GetByID(ctx, tx, 999999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestGuildRepository_GetByName(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	leader := createTestCharacter(t, ctx, tx, "Guild Leader Two")
	repo := NewGuildRepository()

	createdGuild, err := repo.Create(ctx, tx, Guild{
		Name:              "Test Guild Beta",
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	fetchedGuild, err := repo.GetByName(ctx, tx, "Test Guild Beta")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if fetchedGuild.ID != createdGuild.ID {
		t.Errorf("GetByName().ID = %d, want %d", fetchedGuild.ID, createdGuild.ID)
	}

	_, err = repo.GetByName(ctx, tx, "Nonexistent Guild XYZ")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByName() error = %v, want ErrNotFound", err)
	}
}

func TestGuildRepository_List(t *testing.T) {
	tx := beginTx(t)
	ctx := context.Background()
	repo := NewGuildRepository()

	before, err := repo.List(ctx, tx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	leader := createTestCharacter(t, ctx, tx, "Guild Leader Three")
	if _, err := repo.Create(ctx, tx, Guild{Name: "Test Guild Gamma", LeaderCharacterID: leader.ID, LandOfOrigin: "T"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	after, err := repo.List(ctx, tx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("List() len = %d, want %d", len(after), len(before)+1)
	}
}
