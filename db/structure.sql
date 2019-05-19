--
-- structure.sql
--
-- This file describes the structure of the Postgres database used by the
-- application to track its state across runs.
--
-- The overriding directive here is to take advantage of the fact that we're on
-- a strong RDMS. In all cases try to design the schema to make constraints as
-- strong as possible to reduce the likelihood of inconsistencies in our data.
-- For example, use `NOT NULL`, `UNIQUE`, and `REFERENCES` everywhere that it's
-- possible.
--

BEGIN;

DROP TABLE IF EXISTS playlists CASCADE;
DROP TABLE IF EXISTS playlists_songs CASCADE;
DROP TABLE IF EXISTS songs CASCADE;
DROP TABLE IF EXISTS special_playlists CASCADE;

--
-- playlists
--
-- Each playlist is a list of songs that were played at a single night of Death
-- Guild.
--
CREATE TABLE playlists (
    id bigserial PRIMARY KEY,
    day date NOT NULL UNIQUE,
    spotify_id TEXT
);

--
-- special_playlists
--
-- A table that allows us to track the Spotify IDs of "special" playlists --
-- for example, top songs of the year or top songs of all-time.
--
CREATE TABLE special_playlists (
    id bigserial PRIMARY KEY,
    slug TEXT UNIQUE NOT NULL,
    spotify_id TEXT NOT NULL
);

--
-- songs
--
-- Each song is a single artist/title combination. We can't guarantee that
-- there aren't duplicates in here that may have slightly different lettering
-- or parenthesis combinations, and basically depend on the original Death
-- Guild source for good hygiene.
--
CREATE TABLE songs (
    id bigserial PRIMARY KEY,
    artist TEXT NOT NULL,
    title TEXT NOT NULL,
    spotify_checked_at TIMESTAMPTZ,
    spotify_id TEXT
);

ALTER TABLE songs
    ADD CONSTRAINT unique_songs
    UNIQUE (artist, title);

--
-- playlists_songs
--
-- Associates playlists with songs and includes a position to track the order
-- of songs played.
--
CREATE TABLE playlists_songs (
    id bigserial PRIMARY KEY,
    playlists_id BIGINT NOT NULL REFERENCES playlists(id),
    songs_id BIGINT NOT NULL REFERENCES songs(id),
    position INT NOT NULL,
    CHECK (position >= 0)
);

ALTER TABLE playlists_songs
    ADD CONSTRAINT unique_playlists_positions
    UNIQUE (playlists_id, position);

ALTER TABLE playlists_songs
    ADD CONSTRAINT unique_playlists_songs
    UNIQUE (playlists_id, songs_id);

COMMIT;
