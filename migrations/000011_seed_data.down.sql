-- Reverts migration 000011: removes exactly the rows seeded by the up
-- migration, in FK-safe order.
--
-- characters<->guilds is mutually referential (see 000001's comment), so on
-- the way down we first null out the seeded lore characters' guild_id
-- (breaking characters -> guilds), then delete guilds (nothing else still
-- references them), then delete characters (nothing still references them).

-- 1. Listings and inventories for the seeded items.
DELETE FROM listings
WHERE item_id IN (
    SELECT id FROM items WHERE name IN (
        'Iron Dagger', 'Worn Leather Boots', 'Copper Ring', 'Traveler''s Cloak', 'Wooden Shield',
        'Hunting Bow', 'Simple Robe', 'Chainmail Vest', 'Bronze Helmet', 'Leather Gloves',
        'Rusty Sword', 'Steel Buckler', 'Miner''s Pick', 'Fishing Rod', 'Apprentice Wand',
        'Studded Belt', 'Woolen Cap', 'Riding Crop', 'Herbalist''s Satchel', 'Traveling Cloak',
        'Flamebrand Sword', 'Frostbite Cloak', 'Runed Amulet', 'Stormcaller''s Staff',
        'Dragonhide Vest', 'Shadowstep Boots', 'Thundering Warhammer',
        'Soul Reaver', 'Eye of the Dragon Ring', 'Worldbreaker'
    )
);

DELETE FROM inventories
WHERE item_id IN (
    SELECT id FROM items WHERE name IN (
        'Iron Dagger', 'Worn Leather Boots', 'Copper Ring', 'Traveler''s Cloak', 'Wooden Shield',
        'Hunting Bow', 'Simple Robe', 'Chainmail Vest', 'Bronze Helmet', 'Leather Gloves',
        'Rusty Sword', 'Steel Buckler', 'Miner''s Pick', 'Fishing Rod', 'Apprentice Wand',
        'Studded Belt', 'Woolen Cap', 'Riding Crop', 'Herbalist''s Satchel', 'Traveling Cloak',
        'Flamebrand Sword', 'Frostbite Cloak', 'Runed Amulet', 'Stormcaller''s Staff',
        'Dragonhide Vest', 'Shadowstep Boots', 'Thundering Warhammer',
        'Soul Reaver', 'Eye of the Dragon Ring', 'Worldbreaker'
    )
);

-- 2. Gold pouches for the seeded guilds.
DELETE FROM gold_pouches
WHERE guild_id IN (
    SELECT id FROM guilds WHERE name IN (
        'Vorynthax Guild', 'Fellowship of the Grey', 'Illidari Vanguard',
        'Camelot''s Round Table', 'Winterfell Wardens'
    )
);

-- 3. Items.
DELETE FROM items WHERE name IN (
    'Iron Dagger', 'Worn Leather Boots', 'Copper Ring', 'Traveler''s Cloak', 'Wooden Shield',
    'Hunting Bow', 'Simple Robe', 'Chainmail Vest', 'Bronze Helmet', 'Leather Gloves',
    'Rusty Sword', 'Steel Buckler', 'Miner''s Pick', 'Fishing Rod', 'Apprentice Wand',
    'Studded Belt', 'Woolen Cap', 'Riding Crop', 'Herbalist''s Satchel', 'Traveling Cloak',
    'Flamebrand Sword', 'Frostbite Cloak', 'Runed Amulet', 'Stormcaller''s Staff',
    'Dragonhide Vest', 'Shadowstep Boots', 'Thundering Warhammer',
    'Soul Reaver', 'Eye of the Dragon Ring', 'Worldbreaker'
);

-- 4. Break characters -> guilds before dropping guilds. All 10 lore
-- characters (both the leader and the second member of each guild) have
-- guild_id set, not just the leaders.
UPDATE characters SET guild_id = NULL
WHERE name IN (
    'Sauron', 'Strahd von Zarovich',
    'Gandalf the Grey', 'Aragorn',
    'Illidan Stormrage', 'Arthas Menethil',
    'King Arthur', 'Merlin',
    'Jon Snow', 'Daenerys Targaryen'
);

-- 5. Guilds.
DELETE FROM guilds WHERE name IN (
    'Vorynthax Guild', 'Fellowship of the Grey', 'Illidari Vanguard',
    'Camelot''s Round Table', 'Winterfell Wardens'
);

-- 6. Characters (lore + blacksmiths).
DELETE FROM characters WHERE name IN (
    'Sauron', 'Strahd von Zarovich',
    'Gandalf the Grey', 'Aragorn',
    'Illidan Stormrage', 'Arthas Menethil',
    'King Arthur', 'Merlin',
    'Jon Snow', 'Daenerys Targaryen',
    'Grumnus Steelshaper', 'Borgosh Corebender', 'Muraco Bigfoot', 'Thargas Anvilmar',
    'Okothos Ironrager', 'Brannock Coalfist', 'Zarvana Steelclaw', 'Ganthar Deepforge',
    'Doreen Firststrike', 'Krunn Emberforge'
);
