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
	txn, err := db.Begin()
	assert.NoError(t, err)
	defer func() {
		err := txn.Rollback()
		assert.NoError(t, err)
	}()

	songs := []*deathguild.Song{
		{Artist: "Depeche Mode", Title: "Two Minute Warning", SpotifyID: "spotify-id"},
		{Artist: "Imperative Reaction", Title: "You Remain"},
	}

	for _, song := range songs {
		tt.InsertSong(t, txn, song)
	}

	actualSongs, err := songsNeedingID(txn, 1000)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(actualSongs))
	assert.Equal(t, songs[1].ID, actualSongs[0].ID)
}

func TestTrimParenthesis(t *testing.T) {
	assert.Equal(t, "Song", trimParenthesis("Song"))
	assert.Equal(t, "Song", trimParenthesis("Song (So-and-so remix)"))
	assert.Equal(t, "Song", trimParenthesis("Song (So-and-so remix) (Other)"))
}

func TestUpdateSong(t *testing.T) {
	txn, err := db.Begin()
	assert.NoError(t, err)
	defer func() {
		err := txn.Rollback()
		assert.NoError(t, err)
	}()

	song := deathguild.Song{Artist: "Panic Lift", Title: "The Path"}
	tt.InsertSong(t, txn, &song)

	//
	// Should update timestamp but without ID if necessary.
	//

	song.SpotifyCheckedAt = time.Now()

	err = updateSong(txn, &song)
	assert.NoError(t, err)

	var spotifyCheckedAt time.Time
	var spotifyID sql.NullString

	err = txn.QueryRow(`
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

	err = updateSong(txn, &song)
	assert.NoError(t, err)

	err = txn.QueryRow(`
		SELECT spotify_checked_at, spotify_id
		FROM songs
		WHERE id = $1`,
		song.ID,
	).Scan(&spotifyCheckedAt, &spotifyID)

	assert.Equal(t, song.SpotifyCheckedAt.Unix(), spotifyCheckedAt.Unix())
	assert.Equal(t, "spotify-id", spotifyID.String)
}
