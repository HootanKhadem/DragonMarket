-- Table names are pluralized (characters, guilds, items, ...) across every
-- migration in this directory for two reasons: consistency, and because
-- CHARACTER is a reserved SQL keyword that would otherwise need quoting on
-- every query.
--
-- characters.guild_id and guilds.leader_character_id are mutually
-- referential. Resolution: create "characters" first with a plain (FK-less)
-- guild_id column, then create "guilds" referencing "characters" (works,
-- since characters already exists), then add the deferred
-- characters.guild_id -> guilds FK in migration 000003 once both tables
-- exist. Seed/insert order this implies: insert a character with guild_id
-- NULL, insert the guild with leader_character_id set to that character's
-- id, then UPDATE the character's guild_id to point at the new guild.
CREATE TABLE characters (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    land_of_origin TEXT NOT NULL,
    stats TEXT,
    guild_id BIGINT
);

CREATE INDEX idx_characters_guild_id ON characters (guild_id);
