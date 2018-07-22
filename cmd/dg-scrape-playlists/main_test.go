package main

import (
	"os"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/brandur/deathguild"
	tt "github.com/brandur/deathguild/testing"
	assert "github.com/stretchr/testify/require"
)

func init() {
	db = tt.DB
}

func TestExtractDay(t *testing.T) {
	assert.Equal(t,
		"2015-12-21",
		extractDay(PlaylistLink("http://www.deathguild.com//playlist/2015-12-21")),
	)
}

func TestScrapeIndex(t *testing.T) {
	f, err := os.Open("../../testing/samples/playlists.html")
	assert.NoError(t, err)
	defer f.Close()

	links, err := scrapeIndex(f)
	assert.NoError(t, err)

	assert.Equal(t,
		PlaylistLink("http://www.deathguild.com/playlist/1995-10-16"),
		links[len(links)-1],
	)
}

func TestScrapePlaylist(t *testing.T) {
	// Old format
	{
		f, err := os.Open("../../testing/samples/2016-09-26.html")
		assert.NoError(t, err)
		defer f.Close()

		songs, err := scrapePlaylist(f)
		assert.NoError(t, err)

		assert.Equal(t,
			&deathguild.Song{Artist: "Panic Lift", Title: "The Path"},
			songs[len(songs)-1],
		)
	}

	// New format
	{
		f, err := os.Open("../../testing/samples/2018-07-16.html")
		assert.NoError(t, err)
		defer f.Close()

		songs, err := scrapePlaylist(f)
		assert.NoError(t, err)

		assert.Equal(t,
			&deathguild.Song{Artist: "BT", Title: "Godspeed"},
			songs[len(songs)-1],
		)

		// Make sure HTML unescaping in artist names and titles works (I
		// manually added the `&amp;` to the test data in the title here).
		assert.Equal(t,
			&deathguild.Song{Artist: "Simon & Garfunkel", Title: "I Am A Rock &"},
			songs[0],
		)
	}
}

func TestUpsertPlaylistAndSongs(t *testing.T) {
	txn, err := db.Begin()
	assert.NoError(t, err)
	defer func() {
		err := txn.Rollback()
		assert.NoError(t, err)
	}()

	day := "2016-01-01"
	songs := []*deathguild.Song{
		{Artist: "Depeche Mode", Title: "Two Minute Warning"},
		{Artist: "Imperative Reaction", Title: "You Remain"},
	}

	err = upsertPlaylistAndSongs(txn, day, songs)
	assert.NoError(t, err)

	var playlistID string
	err = txn.QueryRow(`
		SELECT id
		FROM playlists
		WHERE day = $1`,
		day,
	).Scan(&playlistID)
	assert.NoError(t, err)

	log.Debugf("New playlist ID is %v", playlistID)
	assert.NotEqual(t, 0, playlistID)

	for _, song := range songs {
		var songID string
		err = txn.QueryRow(`
			SELECT id
			FROM songs
			WHERE artist = $1 AND title = $2`,
			song.Artist, song.Title,
		).Scan(&songID)
		assert.NoError(t, err)

		log.Debugf("New song ID for %v - %v is %v",
			song.Artist, song.Title, songID)
		assert.NotEqual(t, 0, songID)

		var playlistSongID string
		err = txn.QueryRow(`
			SELECT id
			FROM playlists_songs
			WHERE playlists_id = $1 AND songs_id = $2`,
			playlistID, songID,
		).Scan(&playlistSongID)
		assert.NoError(t, err)

		log.Debugf("New playlist/song join ID for %v - %v is %v",
			song.Artist, song.Title, playlistSongID)
		assert.NotEqual(t, 0, songID)
	}
}
