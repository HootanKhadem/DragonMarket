CREATE TYPE listing_status AS ENUM ('ACTIVE', 'EXPIRED');

CREATE TABLE listings (
    id BIGSERIAL PRIMARY KEY,
    item_id BIGINT NOT NULL REFERENCES items (id),
    guild_id BIGINT NOT NULL REFERENCES guilds (id),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    base_price INTEGER NOT NULL CHECK (base_price >= 0),
    status listing_status NOT NULL DEFAULT 'ACTIVE',
    CHECK (status <> 'ACTIVE' OR quantity > 0)
);

CREATE INDEX idx_listings_item_id ON listings (item_id);
CREATE INDEX idx_listings_guild_id ON listings (guild_id);
CREATE INDEX idx_listings_status ON listings (status);
