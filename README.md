# DragonMarket

A guild marketplace for trading fantasy items: fixed-price listings for
COMMON/RARE items, timed auctions for unique LEGENDARY items, gold-pouch
wallets with daily spending limits, and a background price oracle that keeps
legendary item prices current. Built in Go with Gin and Postgres.

## Stack

- Go 1.26, [Gin](https://github.com/gin-gonic/gin) for HTTP
- Postgres 16, accessed via [pgx/v5](https://github.com/jackc/pgx) with hand-written SQL (no ORM)
- [golang-migrate](https://github.com/golang-migrate/migrate) for schema + seed migrations, run automatically on startup
- [patrickmn/go-cache](https://github.com/patrickmn/go-cache) for the in-process legendary-item price cache
- [testcontainers-go](https://github.com/testcontainers/testcontainers-go) for repository/service/e2e tests against a real Postgres

See `docs/adr/0001-architecture.md` for the reasoning behind these choices.

## Running it

Requires Docker and Docker Compose.

```bash
cp .env.example .env   # optional, defaults already work for local dev
docker-compose up --build
```

This starts a Postgres container and the app container. On boot, the app
runs all pending migrations (schema + seed data) against the database, then
starts serving on port 8080 (configurable via `.env`).

Check it's up:

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

The seed migration populates 5 guilds, 20 characters (10 guild-affiliated,
10 unaffiliated blacksmiths), and 30 items (20 COMMON, 7 RARE, 3 LEGENDARY)
with active listings for the COMMON/RARE items, so there's data to hit
immediately:

```bash
curl http://localhost:8080/items
```

### Config

Everything is env-driven (see `.env.example` and `internal/config`):

| Var | Default | Notes |
|---|---|---|
| `DATABASE_URL` | — | required, e.g. `postgres://dragonmarket:dragonmarket@postgres:5432/dragonmarket?sslmode=disable` |
| `PORT` | `8080` | port the app listens on |
| `ORACLE_REFRESH_INTERVAL` | `30s` | how often the price oracle ticker refreshes legendary item prices |
| `AUCTION_SWEEP_INTERVAL` | `10s` | how often the settlement job checks for expired auctions |

## Running the tests

```bash
go test ./...
```

Tests need Docker running — nearly every package (`internal/repository`,
`internal/service`, `internal/settlement`, `internal/migrate`,
`internal/e2e`, ...) spins up a real Postgres container via testcontainers-go
rather than mocking the database, since most of the interesting behavior here
(row locking, generated columns, partial unique indexes) only means anything
against a real Postgres. No manual `docker-compose up` or container setup is
needed — `go test ./...` handles it end to end.

`internal/e2e` is the largest suite: it boots the whole assembled app (real
router, real Postgres, real migrations/seed data, a controllable mock price
oracle) behind an `httptest.Server` and drives every endpoint over real HTTP,
including concurrency races (concurrent purchases against the same listing,
concurrent bids near the 5% floor, a bid landing mid-settlement-sweep) and
auction-lifecycle edge cases.

## API overview

No auth — the acting guild is passed via the `X-Guild-ID` header on routes
that need one. All errors come back as:

```json
{"error": {"code": "SOME_CODE", "message": "human readable message"}}
```

**Items**

- `POST /items` — create an item. `rarity` is `COMMON`/`RARE`/`LEGENDARY`. COMMON/RARE also need `price` and `quantity` (creates an inventory row + an active listing). LEGENDARY items get an inventory row only — no listing, no auction. `guild_id` is optional; omitted defaults to "Vorynthax Guild".
- `GET /items?limit=&offset=` — list items (legendary items show their oracle-cached base price and an `auction_only` flag).
- `GET /items/:id` — item detail.
- `POST /items/:id/purchase` — buy a COMMON/RARE item off its active listing. Requires `X-Guild-ID`.

**Auctions & bids**

- `POST /auctions` — start an auction on a LEGENDARY item you own. Requires `X-Guild-ID`. Body: `item_id`, `duration_seconds`.
- `GET /auctions?limit=&offset=` — list active auctions.
- `GET /auctions/:id` — auction detail.
- `POST /items/:id/bid` — place a bid (must be >= base price for the first bid, or >= 5% above the current highest active bid). Requires `X-Guild-ID`. A bid in the last 5 minutes extends the auction by 5 minutes.
- `DELETE /items/:id/bid/:bid_id` — cancel a bid, unless it's the current highest active bid. Requires `X-Guild-ID`.

**Wallet**

- `GET /guilds/:id/wallet` — a guild's gold pouch: total/reserved/usable balance, daily spending limit, spent today.

### Example: purchase a listed item

```bash
curl -X POST http://localhost:8080/items/2/purchase \
  -H "X-Guild-ID: 1" \
  -H "Content-Type: application/json" \
  -d '{"quantity": 1}'
```

### Example: start an auction and bid on it

```bash
# start an auction (caller must own the legendary item)
curl -X POST http://localhost:8080/auctions \
  -H "X-Guild-ID: 1" \
  -H "Content-Type: application/json" \
  -d '{"item_id": 28, "duration_seconds": 3600}'

# bid on it (amount must be >= the auction's base_price, which tracks the
# item's oracle-cached price -- check GET /items/28 first if this rejects)
curl -X POST http://localhost:8080/items/28/bid \
  -H "X-Guild-ID: 2" \
  -H "Content-Type: application/json" \
  -d '{"amount": 10000}'
```

Auctions past their `end_time` are settled by a background job every
`AUCTION_SWEEP_INTERVAL` (default 10s): the highest bid wins, funds move,
ownership transfers, and losing bids are released.
