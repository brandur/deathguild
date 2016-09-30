package main

import (
	"testing"

	"github.com/brandur/deathguild"
	assert "github.com/stretchr/testify/require"
)

func TestPlaylistInfo(t *testing.T) {
	playlist := &deathguild.Playlist{
		Songs: []*deathguild.Song{
			{Artist: "Depeche Mode", Title: "Two Minute Warning", SpotifyID: "spotify-id"},
			{Artist: "Imperative Reaction", Title: "You Remain"},
		},
		SpotifyID: "spotify-id",
	}

	assert.Equal(t, "2 song(s). 1 song(s) (50.0%) found in Spotify.",
		playlistInfo(playlist))
}

func TestSpotifyPlaylistLink(t *testing.T) {
	conf.SpotifyUser = "fyrerise"
	playlist := &deathguild.Playlist{SpotifyID: "spotify-id"}

	assert.Equal(t, "https://open.spotify.com/user/fyrerise/playlist/spotify-id",
		spotifyPlaylistLink(playlist))
}
