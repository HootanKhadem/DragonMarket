DROP TRIGGER IF EXISTS trg_auctions_set_item_rarity ON auctions;
DROP FUNCTION IF EXISTS set_auction_item_rarity();
DROP INDEX IF EXISTS idx_auctions_active_item_unique;
DROP TABLE IF EXISTS auctions;
DROP TYPE IF EXISTS auction_status;
