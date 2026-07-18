CREATE TYPE transaction_type AS ENUM ('PURCHASE', 'RESERVE', 'RELEASE', 'CREDIT', 'AUCTION_WIN');

-- reference is a free-form string (e.g. "item:12", "auction:4",
-- "listing:7") tying the log entry back to what it was for, per Global
-- Constraints ("must be sufficient to answer 'what was this transaction
-- for'"). Kept as a single nullable text column rather than typed
-- reference_type/reference_id columns since a transaction log's reference
-- has no FK integrity requirement of its own and the referenced row may
-- later be deleted/expired.
CREATE TABLE transaction_logs (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds (id),
    type transaction_type NOT NULL,
    amount INTEGER NOT NULL CHECK (amount >= 0),
    reference TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transaction_logs_guild_id ON transaction_logs (guild_id);
CREATE INDEX idx_transaction_logs_type ON transaction_logs (type);
