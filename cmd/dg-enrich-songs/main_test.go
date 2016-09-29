package main

import (
	"database/sql"
	"testing"
	"time"

	"github.com/brandur/deathguild"
	tt "github.com/brandur/deathguild/testing"
	assert "github.com/stretchr/testify/require"
)

func init() {
	db = tt.DB
}

func TestSongsNeedingID(t *testing.T) {
	tt.TruncateTestDB()

	songs := []*deathguild.Song{
		{Artist: "Depeche Mode", Title: "Two Minute Warning", SpotifyID: "spotify-id"},
		{Artist: "Imperative Reaction", Title: "You Remain"},
	}

	for _, song := range songs {
		insertSong(t, song)
	}

	actualSongs, err := songsNeedingID(1000)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(actualSongs))
	assert.Equal(t, songs[1], actualSongs[0])
}

func TestUpdateSong(t *testing.T) {
	tt.TruncateTestDB()

	song := deathguild.Song{Artist: "Panic Lift", Title: "The Path"}
	insertSong(t, &song)

	//
	// Should update timestamp but without ID if necessary.
	//

	song.SpotifyCheckedAt = time.Now()

	err := updateSong(&song)
	assert.NoError(t, err)

	var spotifyCheckedAt time.Time
	var spotifyID sql.NullString

	err = db.QueryRow(`
		SELECT spotify_checked_at, spotify_id
		FROM songs
		WHERE id = $1`,
		song.ID,
	).Scan(&spotifyCheckedAt, &spotifyID)

	assert.Equal(t, song.SpotifyCheckedAt.Unix(), spotifyCheckedAt.Unix())

	// Checks that the value is indeed NULL.
	assert.Equal(t, false, spotifyID.Valid)

	//
	// And should also be able to update ID when one comes in.
	//

	song.SpotifyID = "spotify-id"

	err = updateSong(&song)
	assert.NoError(t, err)

	err = db.QueryRow(`
		SELECT spotify_checked_at, spotify_id
		FROM songs
		WHERE id = $1`,
		song.ID,
	).Scan(&spotifyCheckedAt, &spotifyID)

	assert.Equal(t, song.SpotifyCheckedAt.Unix(), spotifyCheckedAt.Unix())
	assert.Equal(t, "spotify-id", spotifyID.String)
}

func insertSong(t *testing.T, song *deathguild.Song) {
	var spotifyID *string
	if song.SpotifyID != "" {
		spotifyID = &song.SpotifyID
	}

	err := db.QueryRow(`
		INSERT INTO songs (artist, title, spotify_id)
		VALUES ($1, $2, $3)
		RETURNING id`,
		song.Artist,
		song.Title,
		spotifyID,
	).Scan(&song.ID)
	assert.NoError(t, err)
}
