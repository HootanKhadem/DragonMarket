package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"DragonMarket/internal/migrate"
	"DragonMarket/internal/repository"
)

func init() {
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		return 1
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		return 1
	}

	if err := migrate.Up(connStr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "migrate.Up() failed: %v\n", err)
		return 1
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to parse pool config: %v\n", err)
		return 1
	}
	poolCfg.MaxConns = 20

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to connect pgxpool: %v\n", err)
		return 1
	}
	defer pool.Close()
	testPool = pool

	return m.Run()
}

func beginTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})
	return tx
}

func createTestGuild(t *testing.T, ctx context.Context, db repository.DBTX, name string) repository.Guild {
	t.Helper()
	leader, err := repository.NewCharacterRepository().Create(ctx, db, repository.Character{
		Name:         name + " Leader",
		LandOfOrigin: "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create leader character: %v", err)
	}
	g, err := repository.NewGuildRepository().Create(ctx, db, repository.Guild{
		Name:              name,
		LeaderCharacterID: leader.ID,
		LandOfOrigin:      "Testland",
	})
	if err != nil {
		t.Fatalf("fixture: create guild %q: %v", name, err)
	}
	return g
}

func createTestGoldPouch(t *testing.T, ctx context.Context, db repository.DBTX, gp repository.GoldPouch) repository.GoldPouch {
	t.Helper()
	if gp.LastResetDate.IsZero() {
		gp.LastResetDate = time.Now().UTC().Truncate(24 * time.Hour)
	}
	created, err := repository.NewGoldPouchRepository().Create(ctx, db, gp)
	if err != nil {
		t.Fatalf("fixture: create gold pouch for guild %d: %v", gp.GuildID, err)
	}
	return created
}
