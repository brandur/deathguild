package main

import (
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/lib/pq"
	assert "github.com/stretchr/testify/require"
)

var tablesToTruncate = []string{
	"playlists",
	"playlists_songs",
	"songs",
}

func init() {
	var err error
	db, err = sql.Open("postgres",
		"postgres://localhost/deathguild-test?sslmode=disable")
	if err != nil {
		panic(err)
	}

	for _, table := range tablesToTruncate {
		_, err = db.Exec(`TRUNCATE TABLE ` + table + ` CASCADE`)
		if err != nil {
			panic(err)
		}
	}
}

func TestScrapeIndex(t *testing.T) {
	f, err := os.Open("../../testing/samples/playlists.html")
	assert.NoError(t, err)
	defer f.Close()

	links, err := scrapeIndex(f)
	assert.NoError(t, err)

	assert.Equal(t,
		PlaylistLink("http://www.darkdb.com/deathguild/Playlist.aspx?date=1995-10-16"),
		links[len(links)-1],
	)
}

func TestScrapePlaylist(t *testing.T) {
	f, err := os.Open("../../testing/samples/2016-09-26.html")
	assert.NoError(t, err)
	defer f.Close()

	songs, err := scrapePlaylist(f)
	assert.NoError(t, err)

	assert.Equal(t,
		&Song{"Panic Lift", "The Path"},
		songs[len(songs)-1],
	)
}

func TestUpsertPlaylistAndSongs(t *testing.T) {
	day := "2016-01-01"
	songs := []*Song{
		{"Depeche Mode", "Two Minute Warning"},
		{"Imperative Reaction", "You Remain"},
	}

	err := upsertPlaylistAndSongs(day, songs)
	assert.NoError(t, err)

	var playlistID string
	err = db.QueryRow(`
		SELECT id
		FROM playlists
		WHERE day = $1`,
		day,
	).Scan(&playlistID)
	assert.NoError(t, err)

	log.Printf("New playlist ID is %v", playlistID)
	assert.NotEqual(t, 0, playlistID)

	for _, song := range songs {
		var songID string
		err = db.QueryRow(`
			SELECT id
			FROM songs
			WHERE artist = $1 AND title = $2`,
			song.Artist, song.Title,
		).Scan(&songID)
		assert.NoError(t, err)

		log.Printf("New song ID for %v - %v is %v",
			song.Artist, song.Title, songID)
		assert.NotEqual(t, 0, songID)

		var playlistSongID string
		err = db.QueryRow(`
			SELECT id
			FROM playlists_songs
			WHERE playlists_id = $1 AND songs_id = $2`,
			playlistID, songID,
		).Scan(&playlistSongID)
		assert.NoError(t, err)

		log.Printf("New playlist/song join ID for %v - %v is %v",
			song.Artist, song.Title, playlistSongID)
		assert.NotEqual(t, 0, songID)
	}
}
