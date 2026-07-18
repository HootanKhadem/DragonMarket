-- usable_balance is a STORED generated column (usable = total - reserved)
-- rather than a value maintained by application code, so it can never drift
-- out of sync with total_balance/reserved_balance.
CREATE TABLE gold_pouches (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL UNIQUE REFERENCES guilds (id),
    total_balance INTEGER NOT NULL CHECK (total_balance >= 0),
    reserved_balance INTEGER NOT NULL DEFAULT 0 CHECK (reserved_balance >= 0),
    usable_balance INTEGER GENERATED ALWAYS AS (total_balance - reserved_balance) STORED,
    daily_spending_limit INTEGER NOT NULL CHECK (daily_spending_limit >= 0),
    spent_today INTEGER NOT NULL DEFAULT 0 CHECK (spent_today >= 0),
    last_reset_date DATE NOT NULL,
    CHECK (reserved_balance <= total_balance)
);
