BEGIN;

DROP TABLE IF EXISTS events CASCADE;
DROP TABLE IF EXISTS events_songs CASCADE;
DROP TABLE IF EXISTS songs CASCADE;

--
-- events
--
CREATE TABLE events (
    id bigserial PRIMARY KEY,
    day date UNIQUE
);

--
-- songs
--
CREATE TABLE songs (
    id bigserial PRIMARY KEY,
    artist TEXT,
    title TEXT
);

ALTER TABLE songs
    ADD CONSTRAINT unique_songs
    UNIQUE (artist, title);

--
-- events_songs
--
CREATE TABLE events_songs (
    id bigserial PRIMARY KEY,
    events_id BIGINT REFERENCES events(id),
    songs_id BIGINT REFERENCES songs(id)
);

ALTER TABLE events_songs
    ADD CONSTRAINT unique_events_songs
    UNIQUE (events_id, songs_id);

COMMIT;
