CREATE TYPE bid_status AS ENUM ('ACTIVE', 'CANCELLED', 'WON', 'LOST');

CREATE TABLE bids (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions (id),
    guild_id BIGINT NOT NULL REFERENCES guilds (id),
    amount INTEGER NOT NULL CHECK (amount > 0),
    status bid_status NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bids_auction_id ON bids (auction_id);
CREATE INDEX idx_bids_guild_id ON bids (guild_id);
CREATE INDEX idx_bids_status ON bids (status);
