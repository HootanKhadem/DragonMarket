package repository

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"DragonMarket/internal/migrate"
)

func init() {
	// See internal/migrate/migrate_test.go for why this is gated to windows.
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// testPool is a single pgxpool.Pool shared by every test in this package,
// connected to one Postgres test container
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("dragonmarket_repo_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	if err := migrate.Up(connStr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "migrate.Up() failed: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to connect pgxpool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	testPool = pool

	return
}

// beginTx starts a transaction on the shared testPool and registers a
// rollback via t.Cleanup
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
