-- guild.name is UNIQUE because the app resolves a default guild
-- ("Vorynthax Guild") by name
CREATE TABLE guilds (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    leader_character_id BIGINT NOT NULL REFERENCES characters (id),
    land_of_origin TEXT NOT NULL
);

CREATE INDEX idx_guilds_leader_character_id ON guilds (leader_character_id);
