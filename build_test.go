package main

import (
	"testing"

	"github.com/brandur/deathguild/modules/dgcommon"
	assert "github.com/stretchr/testify/require"
)

func TestPlaylistInfo(t *testing.T) {
	playlist := &dgcommon.Playlist{
		Songs: []*dgcommon.Song{
			{Artist: "Depeche Mode", Title: "Two Minute Warning", SpotifyID: "spotify-id"},
			{Artist: "Imperative Reaction", Title: "You Remain"},
		},
		SpotifyID: "spotify-id",
	}

	assert.Equal(t, "1 out of 2 songs (50.0%) were found in Spotify.",
		playlistInfo(playlist))
}

func TestSpotifyPlaylistLink(t *testing.T) {
	conf.SpotifyUser = "fyrerise"

	assert.Equal(t, "https://open.spotify.com/user/fyrerise/playlist/spotify-id",
		spotifyPlaylistLink("spotify-id"))
}

func TestSpotifySongLink(t *testing.T) {
	assert.Equal(t, "https://open.spotify.com/track/spotify-id",
		spotifySongLink("spotify-id"))
}
