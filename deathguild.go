package deathguild

import (
	"time"
)

// Playlist is a playlist for a single night of Deathguild.
type Playlist struct {
	// Day is the date on which the playlist originally played.
	Day time.Time

	// ID is the local database identifier of the song.
	ID int

	// Songs is an ordered set of songs contained by the playlist.
	Songs []*Song

	// SpotifyID is the canonical ID of the playlist that we created in
	// Spotify.
	SpotifyID string
}

// FormattedDay returns the playlist's date formatted into readable ISO8601.
func (p *Playlist) FormattedDay() string {
	return p.Day.Format("2006-01-02")
}

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
