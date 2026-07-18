-- Seed data (Task 4). Populates the empty schema from Task 3 with the
-- starting data the app needs to be exercised end to end: 5 guilds, 20 lore
-- + blacksmith characters, 30 items (20 COMMON / 7 RARE / 3 LEGENDARY), an
-- inventory + gold pouch per guild, and an ACTIVE listing per non-legendary
-- item. No auctions/bids/transaction_logs rows are seeded (those only ever
-- get created via the app's endpoints, per the plan).
--
-- Idempotency: this file is a normal golang-migrate migration, so
-- golang-migrate's schema_migrations version tracking is what makes
-- re-running the app against an already-seeded database a no-op (Up() skips
-- already-applied versions) -- there is no ON CONFLICT/upsert logic here,
-- deliberately, since none is needed.
--
-- Mutual FK note: characters.guild_id -> guilds and guilds.leader_character_id
-- -> characters are mutually referential (see migration 000001's comment).
-- Each of the 5 guilds below is seeded following the order that implies:
-- insert the leader character with guild_id NULL, insert the guild
-- referencing that character, then insert the guild's second member (guild
-- already exists, no NULL dance needed) and backfill the leader's guild_id.

-- ---------------------------------------------------------------------------
-- 1. Lore characters + 5 guilds
-- ---------------------------------------------------------------------------

-- Guild 1: Vorynthax Guild (the default-guild fallback target for
-- POST /items per plan Task 8 -- name must be exactly this).
INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('Sauron', 'Mordor', 'Str: +5, intl: +5, con: +5', NULL);
INSERT INTO guilds (name, leader_character_id, land_of_origin)
    SELECT 'Vorynthax Guild', id, 'The Blighted Wastes' FROM characters WHERE name = 'Sauron';
INSERT INTO characters (name, land_of_origin, stats, guild_id)
    SELECT 'Strahd von Zarovich', 'Barovia', 'Str: +4, intl: +6, con: +5', id
    FROM guilds WHERE name = 'Vorynthax Guild';
UPDATE characters SET guild_id = (SELECT id FROM guilds WHERE name = 'Vorynthax Guild')
    WHERE name = 'Sauron';

-- Guild 2: Fellowship of the Grey (Middle-earth).
INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('Gandalf the Grey', 'Valinor', 'Str: +2, intl: +6, con: +1', NULL);
INSERT INTO guilds (name, leader_character_id, land_of_origin)
    SELECT 'Fellowship of the Grey', id, 'Middle-earth' FROM characters WHERE name = 'Gandalf the Grey';
INSERT INTO characters (name, land_of_origin, stats, guild_id)
    SELECT 'Aragorn', 'Gondor', 'Str: +5, intl: +2, con: +4', id
    FROM guilds WHERE name = 'Fellowship of the Grey';
UPDATE characters SET guild_id = (SELECT id FROM guilds WHERE name = 'Fellowship of the Grey')
    WHERE name = 'Gandalf the Grey';

-- Guild 3: Illidari Vanguard (Outland / Warcraft).
INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('Illidan Stormrage', 'Outland', 'Str: +3, intl: +6, con: +5', NULL);
INSERT INTO guilds (name, leader_character_id, land_of_origin)
    SELECT 'Illidari Vanguard', id, 'Outland' FROM characters WHERE name = 'Illidan Stormrage';
INSERT INTO characters (name, land_of_origin, stats, guild_id)
    SELECT 'Arthas Menethil', 'Northrend', 'Str: +3, intl: +6, con: +4', id
    FROM guilds WHERE name = 'Illidari Vanguard';
UPDATE characters SET guild_id = (SELECT id FROM guilds WHERE name = 'Illidari Vanguard')
    WHERE name = 'Illidan Stormrage';

-- Guild 4: Camelot's Round Table (Arthurian legend).
INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('King Arthur', 'Camelot', 'Str: +5, intl: +3, con: +5', NULL);
INSERT INTO guilds (name, leader_character_id, land_of_origin)
    SELECT 'Camelot''s Round Table', id, 'Camelot' FROM characters WHERE name = 'King Arthur';
INSERT INTO characters (name, land_of_origin, stats, guild_id)
    SELECT 'Merlin', 'Camelot', 'Str: +1, intl: +7, con: +2', id
    FROM guilds WHERE name = 'Camelot''s Round Table';
UPDATE characters SET guild_id = (SELECT id FROM guilds WHERE name = 'Camelot''s Round Table')
    WHERE name = 'King Arthur';

-- Guild 5: Winterfell Wardens (A Song of Ice and Fire).
INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('Jon Snow', 'Winterfell', 'Str: +4, intl: +3, con: +4', NULL);
INSERT INTO guilds (name, leader_character_id, land_of_origin)
    SELECT 'Winterfell Wardens', id, 'The North' FROM characters WHERE name = 'Jon Snow';
INSERT INTO characters (name, land_of_origin, stats, guild_id)
    SELECT 'Daenerys Targaryen', 'Dragonstone', 'Str: +2, intl: +5, con: +3', id
    FROM guilds WHERE name = 'Winterfell Wardens';
UPDATE characters SET guild_id = (SELECT id FROM guilds WHERE name = 'Winterfell Wardens')
    WHERE name = 'Jon Snow';

-- ---------------------------------------------------------------------------
-- 2. Blacksmith characters (Warcraft-flavored), unaffiliated (guild_id NULL)
-- ---------------------------------------------------------------------------

INSERT INTO characters (name, land_of_origin, stats, guild_id) VALUES
    ('Grumnus Steelshaper', 'Ironforge', 'Str: +5, intl: +3, con: +5', NULL),
    ('Borgosh Corebender', 'Orgrimmar', 'Str: +6, intl: +2, con: +5', NULL),
    ('Muraco Bigfoot', 'Thunder Bluff', 'Str: +6, intl: +2, con: +6', NULL),
    ('Thargas Anvilmar', 'Stormwind', 'Str: +4, intl: +3, con: +4', NULL),
    ('Okothos Ironrager', 'Silvermoon City', 'Str: +2, intl: +6, con: +3', NULL),
    ('Brannock Coalfist', 'Kharanos', 'Str: +5, intl: +3, con: +5', NULL),
    ('Zarvana Steelclaw', 'Undercity', 'Str: +3, intl: +4, con: +3', NULL),
    ('Ganthar Deepforge', 'Grim Batol', 'Str: +5, intl: +3, con: +5', NULL),
    ('Doreen Firststrike', 'Darnassus', 'Str: +3, intl: +5, con: +3', NULL),
    ('Krunn Emberforge', 'Blackrock Mountain', 'Str: +5, intl: +2, con: +5', NULL);

-- ---------------------------------------------------------------------------
-- 3. Gold pouches, one per guild
-- ---------------------------------------------------------------------------
-- total_balance: a generous flat starting balance so every seeded guild can
-- act (buy/list/bid) immediately in manual testing without a top-up step.
-- daily_spending_limit: "random integer in [2000, 10000], rounded up to the
-- nearest 100" per the brief -- computed in SQL since this is a one-time
-- seed (idiomatic for a Postgres seed migration, no need for a Go-side RNG).
-- last_reset_date: seed date (today, in the migration's execution context).

INSERT INTO gold_pouches (guild_id, total_balance, reserved_balance, daily_spending_limit, spent_today, last_reset_date)
SELECT g.id,
       50000,
       0,
       (CEIL((2000 + FLOOR(random() * 8001)) / 100.0) * 100)::INTEGER,
       0,
       CURRENT_DATE
FROM guilds g;

-- ---------------------------------------------------------------------------
-- 4. Items -- staged in a helper table first
-- ---------------------------------------------------------------------------
-- _seed_items is a scratch table (dropped at the end of this migration) that
-- holds the 27 COMMON/RARE items' full seed rows in one place, so the later
-- inventories/listings inserts can join against a single source of truth
-- instead of repeating each item's name/quantity/guild literals three times.

CREATE TABLE _seed_items (
    name             TEXT,
    land_of_origin   TEXT,
    rarity           TEXT,
    price            INTEGER,
    forger_name      TEXT,
    quantity         INTEGER,
    owner_guild_name TEXT
);

INSERT INTO _seed_items (name, land_of_origin, rarity, price, forger_name, quantity, owner_guild_name) VALUES
    -- COMMON (20)
    ('Iron Dagger', 'Elwynn Forest', 'COMMON', 15, 'Thargas Anvilmar', 30, 'Vorynthax Guild'),
    ('Worn Leather Boots', 'Duskwood', 'COMMON', 12, 'Doreen Firststrike', 25, 'Fellowship of the Grey'),
    ('Copper Ring', 'Westfall', 'COMMON', 8, 'Grumnus Steelshaper', 40, 'Illidari Vanguard'),
    ('Traveler''s Cloak', 'Loch Modan', 'COMMON', 10, 'Zarvana Steelclaw', 20, 'Camelot''s Round Table'),
    ('Wooden Shield', 'Redridge Mountains', 'COMMON', 18, 'Krunn Emberforge', 15, 'Winterfell Wardens'),
    ('Hunting Bow', 'Ashenvale', 'COMMON', 22, 'Muraco Bigfoot', 18, 'Vorynthax Guild'),
    ('Simple Robe', 'Tirisfal Glades', 'COMMON', 9, 'Okothos Ironrager', 35, 'Fellowship of the Grey'),
    ('Chainmail Vest', 'Dun Morogh', 'COMMON', 28, 'Borgosh Corebender', 12, 'Illidari Vanguard'),
    ('Bronze Helmet', 'Silverpine Forest', 'COMMON', 20, 'Ganthar Deepforge', 22, 'Camelot''s Round Table'),
    ('Leather Gloves', 'Stonetalon Mountains', 'COMMON', 11, 'Brannock Coalfist', 30, 'Winterfell Wardens'),
    ('Rusty Sword', 'The Barrens', 'COMMON', 14, 'Thargas Anvilmar', 26, 'Vorynthax Guild'),
    ('Steel Buckler', 'Durotar', 'COMMON', 19, 'Doreen Firststrike', 17, 'Fellowship of the Grey'),
    ('Miner''s Pick', 'Dun Morogh', 'COMMON', 13, 'Grumnus Steelshaper', 24, 'Illidari Vanguard'),
    ('Fishing Rod', 'Loch Modan', 'COMMON', 7, 'Zarvana Steelclaw', 33, 'Camelot''s Round Table'),
    ('Apprentice Wand', 'Silvermoon City', 'COMMON', 16, 'Krunn Emberforge', 21, 'Winterfell Wardens'),
    ('Studded Belt', 'Mulgore', 'COMMON', 10, 'Muraco Bigfoot', 28, 'Vorynthax Guild'),
    ('Woolen Cap', 'Elwynn Forest', 'COMMON', 6, 'Okothos Ironrager', 40, 'Fellowship of the Grey'),
    ('Riding Crop', 'Thousand Needles', 'COMMON', 9, 'Borgosh Corebender', 19, 'Illidari Vanguard'),
    ('Herbalist''s Satchel', 'Un''Goro Crater', 'COMMON', 12, 'Ganthar Deepforge', 23, 'Camelot''s Round Table'),
    ('Traveling Cloak', 'Felwood', 'COMMON', 11, 'Brannock Coalfist', 27, 'Winterfell Wardens'),
    -- RARE (7)
    ('Flamebrand Sword', 'Blasted Lands', 'RARE', 450, 'Thargas Anvilmar', 6, 'Vorynthax Guild'),
    ('Frostbite Cloak', 'Winterspring', 'RARE', 380, 'Doreen Firststrike', 8, 'Fellowship of the Grey'),
    ('Runed Amulet', 'Azshara', 'RARE', 500, 'Grumnus Steelshaper', 5, 'Illidari Vanguard'),
    ('Stormcaller''s Staff', 'Stonetalon Mountains', 'RARE', 620, 'Zarvana Steelclaw', 4, 'Camelot''s Round Table'),
    ('Dragonhide Vest', 'Un''Goro Crater', 'RARE', 700, 'Krunn Emberforge', 3, 'Winterfell Wardens'),
    ('Shadowstep Boots', 'Felwood', 'RARE', 410, 'Muraco Bigfoot', 7, 'Vorynthax Guild'),
    ('Thundering Warhammer', 'Blackrock Mountain', 'RARE', 750, 'Okothos Ironrager', 4, 'Fellowship of the Grey');

-- 4a. Insert the 27 COMMON/RARE items from the staging table.
INSERT INTO items (name, land_of_origin, rarity, forger_character_id, price)
SELECT s.name, s.land_of_origin, s.rarity::item_rarity, c.id, s.price
FROM _seed_items s
JOIN characters c ON c.name = s.forger_name;

-- 4b. The 3 LEGENDARY items. Soul Reaver and Eye of the Dragon Ring are the
-- two named-by-spec legendaries and are (separately, below) both placed in
-- Vorynthax Guild's inventory. Worldbreaker is the third legendary and is
-- randomly assigned to one of the *other* 4 guilds' inventory below.
INSERT INTO items (name, land_of_origin, rarity, forger_character_id, price) VALUES
    ('Soul Reaver', 'Barovia', 'LEGENDARY', (SELECT id FROM characters WHERE name = 'Ganthar Deepforge'), 8000),
    ('Eye of the Dragon Ring', 'Dragonstone', 'LEGENDARY', (SELECT id FROM characters WHERE name = 'Brannock Coalfist'), 9000),
    ('Worldbreaker', 'The Sundered Peaks', 'LEGENDARY', (SELECT id FROM characters WHERE name = 'Borgosh Corebender'), 8500);

-- ---------------------------------------------------------------------------
-- 5. Inventories
-- ---------------------------------------------------------------------------

-- 5a. One inventory row per COMMON/RARE item, in its designated owner guild.
INSERT INTO inventories (guild_id, item_id, quantity)
SELECT g.id, i.id, s.quantity
FROM _seed_items s
JOIN items i ON i.name = s.name
JOIN guilds g ON g.name = s.owner_guild_name;

-- 5b. Soul Reaver and Eye of the Dragon Ring: both owned by Vorynthax Guild.
INSERT INTO inventories (guild_id, item_id, quantity)
SELECT (SELECT id FROM guilds WHERE name = 'Vorynthax Guild'), i.id, 1
FROM items i
WHERE i.name IN ('Soul Reaver', 'Eye of the Dragon Ring');

-- 5c. Worldbreaker: randomly assigned to one of the other 4 guilds.
INSERT INTO inventories (guild_id, item_id, quantity)
SELECT pick.id, i.id, 1
FROM items i
CROSS JOIN LATERAL (
    SELECT id FROM guilds WHERE name <> 'Vorynthax Guild' ORDER BY random() LIMIT 1
) AS pick
WHERE i.name = 'Worldbreaker';

-- ---------------------------------------------------------------------------
-- 6. Listings -- one ACTIVE listing per COMMON/RARE item, mirroring its
-- inventory row (same owning guild and quantity), so seeded listings are
-- immediately purchasable per the plan's Global Constraints. LEGENDARY items
-- deliberately get no listing (and no auction -- those are only created via
-- POST /auctions, out of scope for this task).
-- ---------------------------------------------------------------------------

INSERT INTO listings (item_id, guild_id, quantity, base_price, status)
SELECT inv.item_id, inv.guild_id, inv.quantity, i.price, 'ACTIVE'
FROM inventories inv
JOIN items i ON i.id = inv.item_id
WHERE i.rarity IN ('COMMON', 'RARE');

-- ---------------------------------------------------------------------------
-- 7. Cleanup
-- ---------------------------------------------------------------------------

DROP TABLE _seed_items;
