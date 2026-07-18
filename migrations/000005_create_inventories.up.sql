-- inventories.item_rarity is a denormalized copy of items.rarity, kept in
-- sync by the trigger below. It exists solely so a partial unique index can
-- enforce the Global Constraints rule "a LEGENDARY item lives in at most one
-- guild's inventory at a time": Postgres partial-index predicates (and CHECK
-- constraints) can only reference columns of the table being constrained,
-- not a joined table's columns, so items.rarity can't be consulted directly
-- from an index/check on inventories.
CREATE TABLE inventories (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds (id),
    item_id BIGINT NOT NULL REFERENCES items (id),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    item_rarity item_rarity NOT NULL,
    UNIQUE (guild_id, item_id),
    CHECK (item_rarity <> 'LEGENDARY' OR quantity = 1)
);

CREATE INDEX idx_inventories_item_id ON inventories (item_id);

CREATE OR REPLACE FUNCTION set_inventory_item_rarity() RETURNS TRIGGER AS $$
BEGIN
    SELECT rarity INTO NEW.item_rarity FROM items WHERE id = NEW.item_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_inventories_set_item_rarity
    BEFORE INSERT OR UPDATE OF item_id ON inventories
    FOR EACH ROW
    EXECUTE FUNCTION set_inventory_item_rarity();

-- At most one inventory row (i.e. one owning guild) per LEGENDARY item.
CREATE UNIQUE INDEX idx_inventories_legendary_unique
    ON inventories (item_id)
    WHERE item_rarity = 'LEGENDARY';
