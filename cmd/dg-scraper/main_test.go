package main

import (
	"os"
	"testing"

	assert "github.com/stretchr/testify/require"
)

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
