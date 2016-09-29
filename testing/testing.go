package testing

import (
	"database/sql"
	"testing"

	"github.com/brandur/deathguild"
	assert "github.com/stretchr/testify/require"

	// This package provides the general database infrastructure for the other
	// packages in the project and therefore we pull in pq. This comment is
	// here to satisfy the Linter.
	_ "github.com/lib/pq"
)

var tablesToTruncate = []string{
	"playlists",
	"playlists_songs",
	"songs",
}

// DB references a testing database that can be used in the tests for any
// modules that need a database connection.
var DB *sql.DB

func init() {
	var err error
	DB, err = sql.Open("postgres",
		"postgres://localhost/deathguild-test?sslmode=disable")
	if err != nil {
		panic(err)
	}

	// All tests should be using transactions and roll themselves back, but do
	// an initial clean on the database anyway to remove anything that may
	// have accumulated.
	truncateTestDB(DB)
}

// InsertPlaylist puts a playlist into the database.
func InsertPlaylist(t *testing.T, txn *sql.Tx, playlist *deathguild.Playlist) {
	var spotifyID *string
	if playlist.SpotifyID != "" {
		spotifyID = &playlist.SpotifyID
	}

	err := txn.QueryRow(`
		INSERT INTO playlists (day, spotify_id)
		VALUES ($1, $2)
		RETURNING id`,
		playlist.Day,
		spotifyID,
	).Scan(&playlist.ID)
	assert.NoError(t, err)
}

// InsertSong puts a song into the database.
func InsertSong(t *testing.T, txn *sql.Tx, song *deathguild.Song) {
	var spotifyID *string
	if song.SpotifyID != "" {
		spotifyID = &song.SpotifyID
	}

	err := txn.QueryRow(`
		INSERT INTO songs (artist, title, spotify_id)
		VALUES ($1, $2, $3)
		RETURNING id`,
		song.Artist,
		song.Title,
		spotifyID,
	).Scan(&song.ID)
	assert.NoError(t, err)
}

// truncateTestDB truncates all tables in the testing database.
func truncateTestDB(db *sql.DB) {
	for _, table := range tablesToTruncate {
		_, err := DB.Exec(`TRUNCATE TABLE ` + table + ` CASCADE`)
		if err != nil {
			panic(err)
		}
	}
}
