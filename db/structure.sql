BEGIN;

DROP TABLE IF EXISTS playlists CASCADE;
DROP TABLE IF EXISTS playlists_songs CASCADE;
DROP TABLE IF EXISTS songs CASCADE;

--
-- playlists
--
CREATE TABLE playlists (
    id bigserial PRIMARY KEY,
    day date NOT NULL UNIQUE,
    spotify_id TEXT
);

--
-- songs
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
