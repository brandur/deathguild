package deathguild

// Song is an artist/title pair that we've extracted from a playlist.
type Song struct {
	// Artist is the name of the song's artist.
	Artist string

	// Title is the title of the song.
	Title string

	// SpotifyID is the canonical ID of the song according to Spotify.
	SpotifyID string
}
