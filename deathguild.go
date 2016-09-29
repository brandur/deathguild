package deathguild

import (
	"time"
)

// Song is an artist/title pair that we've extracted from a playlist.
type Song struct {
	// Artist is the name of the song's artist.
	Artist string

	// ID is the local database identifier of the song.
	ID int

	// Title is the title of the song.
	Title string

	// SpotifyCheckedAt is the last time we tried to pull information on the
	// track from Spotify.
	SpotifyCheckedAt time.Time

	// SpotifyID is the canonical ID of the song according to Spotify.
	SpotifyID string
}
