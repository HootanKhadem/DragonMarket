CREATE TYPE auction_status AS ENUM ('ACTIVE', 'EXPIRED');

-- auctions.item_rarity is a denormalized copy of items.rarity, kept in sync
-- by the trigger below, so a CHECK constraint can enforce the Global
-- Constraints rule "auction.item_id must be LEGENDARY". Same technique as
-- inventories.item_rarity in migration 000005, for the same reason: a CHECK
-- constraint (like a partial index) can only reference columns of the table
-- it's defined on, not a joined table's columns.
CREATE TABLE auctions (
    id BIGSERIAL PRIMARY KEY,
    item_id BIGINT NOT NULL REFERENCES items (id),
    owner_guild_id BIGINT NOT NULL REFERENCES guilds (id),
    status auction_status NOT NULL DEFAULT 'ACTIVE',
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    base_price INTEGER NOT NULL CHECK (base_price >= 0),
    item_rarity item_rarity NOT NULL,
    CHECK (end_time > start_time),
    CHECK (item_rarity = 'LEGENDARY')
);

CREATE INDEX idx_auctions_item_id ON auctions (item_id);
CREATE INDEX idx_auctions_owner_guild_id ON auctions (owner_guild_id);

-- At most one ACTIVE auction per item at a time (Global Constraints).
CREATE UNIQUE INDEX idx_auctions_active_item_unique
    ON auctions (item_id)
    WHERE status = 'ACTIVE';

CREATE OR REPLACE FUNCTION set_auction_item_rarity() RETURNS TRIGGER AS $$
BEGIN
    SELECT rarity INTO NEW.item_rarity FROM items WHERE id = NEW.item_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_auctions_set_item_rarity
    BEFORE INSERT OR UPDATE OF item_id ON auctions
    FOR EACH ROW
    EXECUTE FUNCTION set_auction_item_rarity();
