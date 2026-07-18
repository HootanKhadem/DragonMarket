CREATE TYPE listing_status AS ENUM ('ACTIVE', 'EXPIRED');

-- quantity must never be negative, and an ACTIVE listing must have
-- quantity > 0 (a zero-quantity listing is domain-nonsensical while still
-- being sold) — but quantity = 0 is exactly the terminal state a listing
-- reaches when fully purchased, at which point Task 8's purchase flow flips
-- it to EXPIRED in the same update (see plan Task 8: "Listing -> EXPIRED at
-- 0"), so plain `quantity > 0` would wrongly reject that transition.
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
