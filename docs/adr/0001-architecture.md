# 1. Architecture of the DragonMarket marketplace

## Status

Accepted.

## Context

DragonMarket is a guild marketplace: guilds trade COMMON/RARE items at fixed
prices and bid on unique LEGENDARY items via timed auctions, all funded from
per-guild gold pouches with daily spending limits. The core hard problem
isn't the HTTP layer — it's correctness under concurrency: two guilds must
never buy the last unit of the same listing, two guilds must never both win
the same auction, and a gold pouch's `usable_balance` must never drift or go
negative no matter how many requests hit it at once. Everything below was
chosen with that problem in mind, not because it's fashionable.

## Decisions

### Gin for HTTP

Gin was already scaffolded before the harder decisions below were made, and
there was never a reason to reconsider it: the routing and middleware needs
here (path params, a couple of Gin-level middlewares for `X-Guild-ID` and
error formatting) are simple, and a heavier framework would add nothing.
Trade-off: Gin's ecosystem conventions (e.g. `c.ShouldBindJSON`, `gin.H`)
leak a little into handler code, but that's a small, well-understood cost.

### pgx + hand-written SQL over an ORM

Every write path in this system is a multi-row, lock-then-check-then-mutate
transaction (reserve funds, decrement inventory, expire a listing, write a
transaction log — all atomically). An ORM's value proposition is mostly
about hiding SQL for simple CRUD; here the SQL *is* the business logic, and
hiding it would make the locking behavior harder to read and reason about,
not easier. pgx/v5 also gives direct access to `pgx.Tx` and `SELECT ... FOR
UPDATE`, which the whole gold-pouch/auction design depends on. Trade-off:
more boilerplate per query, and schema changes touch hand-written SQL in
more places than a model-first ORM would — an acceptable cost for a system
this size, and one that keeps the actual locking semantics visible in the
repository code rather than behind an abstraction.

### golang-migrate for schema + seed data

Plain numbered up/down SQL migrations, run automatically at app startup
before serving traffic. Migrations double as documentation of the schema's
evolution, and the DB-level constraints (CHECK constraints, partial unique
indexes, the denormalized-column-plus-trigger tricks — see below) all live
naturally as migration SQL rather than needing a separate schema-management
story. The seed data (5 guilds, characters, 30 items, listings) is migration
`000011`, which gets golang-migrate's up/down tracking (and idempotency) for
free instead of needing a bespoke seeding mechanism.

### In-process cache (go-cache) over Redis

Legendary item prices are refreshed by a single background ticker inside the
one running app instance and read by that same instance's request handlers
— there's no cross-instance cache-sharing need to justify Redis's
operational cost (another service to run, deploy, and monitor) for a
single-process app. `patrickmn/go-cache` is a simple in-memory map with
expiry, which is exactly the shape of "last known-good price per item ID"
this needs. Trade-off: this doesn't scale past one instance without
switching to something shared — a real constraint if this app is ever
horizontally scaled, noted here rather than solved for prematurely.

### Row-locking (`SELECT ... FOR UPDATE`) over optimistic concurrency

Every balance/inventory/auction mutation acquires a row lock inside a
transaction before checking-then-writing. Optimistic concurrency (version
columns, retry-on-conflict) was considered, but under contention — many
guilds bidding on the same popular auction, or racing the last unit of a
listing — optimistic retries would mean rejecting and re-driving requests
that pessimistic locking just serializes cleanly the first time. Given how
central "never double-spend, never double-sell" is to this domain, the
simplicity of "lock the row, check the invariant, mutate, commit" was worth
more than the extra throughput headroom optimistic concurrency can offer
under low contention. This is proven out directly by concurrency tests in
`internal/service`, `internal/settlement`, and `internal/e2e` (concurrent
reservations against one gold pouch, concurrent bids near the 5% floor,
concurrent purchases against one listing).

### Background tickers (goroutines) over a job-scheduling framework

The price oracle refresh (every `ORACLE_REFRESH_INTERVAL`, default 30s) and
auction settlement sweep (every `AUCTION_SWEEP_INTERVAL`, default 10s) are
both just `context.Context` + `time.Ticker` in a goroutine, started once in
`main.go`. There's no need for a distributed job scheduler, retry queues, or
persistence of job state — these are two fixed-interval, idempotent sweeps
against the same database the app already talks to, and a framework here
would add operational surface (another system to configure and monitor) for
no real benefit at this scale.

### testcontainers-go for tests

Nearly every package that touches Postgres (`internal/repository`,
`internal/service`, `internal/settlement`, `internal/migrate`,
`internal/e2e`) tests against a real, ephemeral Postgres container rather
than mocking the database. The behavior under test — generated columns,
partial unique indexes, row locking, transaction semantics — either doesn't
exist in a mock or behaves subtly differently, so a mock would be testing
the mock's model of Postgres, not Postgres. `go test ./...` spins up and
tears down containers automatically; no manual database setup is needed to
run the suite.

### Why Postgres specifically

Postgres wasn't just "the SQL database at hand" — several of its specific
features are load-bearing for this design:

- **Relational integrity for the financial/ownership invariants.** Foreign
  keys keep items/inventories/auctions/bids honest about what references
  what. CHECK constraints enforce non-negative balances/quantities/prices
  and `reserved_balance <= total_balance` directly in the schema, so a bug
  in application code can't silently push a gold pouch negative. Legendary
  uniqueness ("at most one guild owns this legendary item") and "at most one
  ACTIVE auction per item" are both enforced with partial unique indexes —
  `inventories(item_id) WHERE item_rarity = 'LEGENDARY'` and
  `auctions(item_id) WHERE status = 'ACTIVE'` — backed by a small trigger
  that denormalizes `items.rarity` onto each row, since a partial index
  predicate can only reference columns on its own table. These are exactly
  the kind of invariant that's easy to get right in code once and then
  quietly violate later; having the database refuse to store the bad state
  is a much stronger guarantee.
- **Real transactions and row-level locking**, which the entire gold-pouch
  and auction-settlement design depends on. `SELECT ... FOR UPDATE` inside a
  transaction is what makes "reserve funds, then insert a bid, then maybe
  extend the auction" atomic and race-free against concurrent bidders — this
  isn't a nice-to-have, it's the mechanism the whole concurrency story rests
  on.
- **A GENERATED column for `usable_balance`** (`total_balance -
  reserved_balance`, `STORED`), so it's physically impossible for it to
  drift out of sync with the two balances it's derived from — no
  application code path can update one without the other.
- **A mature Go driver ecosystem.** pgx/v5 is a well-maintained, actively
  developed driver with first-class `pgxpool` connection pooling and a
  `pgx.Tx` type that composes cleanly through the repository layer — the
  `DBTX` interface (satisfied by both `*pgxpool.Pool` and `pgx.Tx`) is what
  lets service-layer code hand either one to a repository method depending
  on whether it's inside a locked transaction or just reading.

For a marketplace whose entire value proposition is "never let two guilds
end up owning the same legendary sword," a database that can enforce that at
the schema level, not just in application code, is the right fit.

## Consequences

- Business logic is easier to trust because a meaningful slice of it (no
  negative balances, no double-owned legendaries, no two active auctions on
  one item) is enforced by Postgres itself, not just by careful application
  code.
- The test suite is slower than a pure-mock suite would be (containers take
  real seconds to start), but it tests real behavior — the trade was made
  deliberately and is paid back every time a locking or constraint bug is
  caught before it reaches a human.
- Scaling beyond one app instance would need the in-process price cache
  replaced with something shared (Redis or similar), and the two tickers
  would need a leader-election or advisory-lock story so only one instance
  runs each sweep — neither is a problem yet, but both are direct
  consequences of the choices above and are worth flagging for whoever
  scales this next.

## What I would add with more time

1. Authentication for guilds and marketplace admins with role-based access.
2. Admin access for managing the marketplace (characters, items, etc.).
3. An activity log of all requests/responses as a separate service backed by a NoSQL database.
4. Copper/silver/platinum currency denominations.
5. A marketplace auction fee — winner pays x% of the sold item's value as a fee.
6. The ability for guilds to stake funds in the marketplace for an x% profit.
7. A loyalty program giving long-history/high-volume guilds a discount on the marketplace fee.
