-- Deferred half of the characters<->guilds mutual FK (see 000001's comment).
ALTER TABLE characters
    ADD CONSTRAINT fk_characters_guild
    FOREIGN KEY (guild_id) REFERENCES guilds (id);
