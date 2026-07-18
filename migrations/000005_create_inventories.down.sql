DROP INDEX IF EXISTS idx_inventories_legendary_unique;
DROP TRIGGER IF EXISTS trg_inventories_set_item_rarity ON inventories;
DROP FUNCTION IF EXISTS set_inventory_item_rarity();
DROP TABLE IF EXISTS inventories;
