CREATE TYPE item_rarity AS ENUM ('COMMON', 'RARE', 'LEGENDARY');

CREATE TABLE items (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    land_of_origin TEXT NOT NULL,
    rarity item_rarity NOT NULL,
    forger_character_id BIGINT NOT NULL REFERENCES characters (id),
    price INTEGER NOT NULL CHECK (price >= 0)
);

CREATE INDEX idx_items_forger_character_id ON items (forger_character_id);
CREATE INDEX idx_items_rarity ON items (rarity);
