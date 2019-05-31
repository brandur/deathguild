package main

import (
	"database/sql"
	"testing"
	"time"

	"github.com/brandur/deathguild/modules/dgcommon"
	"github.com/brandur/deathguild/modules/dgtesting"
	assert "github.com/stretchr/testify/require"
)

func init() {
	db = dgtesting.DB
}

func TestGetPlaylistsInTransaction(t *testing.T) {
	txn, err := db.Begin()
	assert.NoError(t, err)
	defer func() {
		err := txn.Rollback()
		assert.NoError(t, err)
	}()

	playlists := []*dgcommon.Playlist{
		{Day: time.Now(), SpotifyID: "spotify-id"},
		{Day: time.Now().Add(30 * 24 * time.Hour)},
	}

	for _, playlist := range playlists {
		dgtesting.InsertPlaylist(t, txn, playlist)
	}

	actualPlaylist, err := getPlaylistsInTransaction(txn, 1000)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(actualPlaylist))
	assert.Equal(t, playlists[1].ID, actualPlaylist[0].ID)
}

func TestUpdatePlaylist(t *testing.T) {
	txn, err := db.Begin()
	assert.NoError(t, err)
	defer func() {
		err := txn.Rollback()
		assert.NoError(t, err)
	}()

	playlist := dgcommon.Playlist{Day: time.Now()}
	dgtesting.InsertPlaylist(t, txn, &playlist)

	//
	// Should update without ID if necessary.
	//

	err = updatePlaylist(txn, &playlist)
	assert.NoError(t, err)

	var spotifyID sql.NullString

	err = txn.QueryRow(`
		SELECT spotify_id
		FROM playlists
		WHERE id = $1`,
		playlist.ID,
	).Scan(&spotifyID)

	// Checks that the value is indeed NULL.
	assert.Equal(t, false, spotifyID.Valid)

	//
	// And should also be able to update ID when one comes in.
	//

	playlist.SpotifyID = "spotify-id"

	err = updatePlaylist(txn, &playlist)
	assert.NoError(t, err)

	err = txn.QueryRow(`
		SELECT spotify_id
		FROM playlists
		WHERE id = $1`,
		playlist.ID,
	).Scan(&spotifyID)

	assert.Equal(t, "spotify-id", spotifyID.String)
}
