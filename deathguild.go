package deathguild

import (
	"database/sql"
	"os"
	"path"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brandur/sorg/pool"
)

const (
	// Release allows CSS and JS assets to be invalidated quickly by changing
	// their URL. Bump this number whenever something significant changes that
	// should be invalidated as quickly as possible.
	Release = "1"
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

// FetchSongs populates the playlist's songs collection from the database.
func (p *Playlist) FetchSongs(txn *sql.Tx) error {
	// Add one to position to make it 1-indexed as people are more used to
	// that.
	rows, err := txn.Query(`
		SELECT s.id, (position + 1), artist, title, spotify_checked_at, spotify_id
		FROM playlists_songs ps
		INNER JOIN songs s ON ps.songs_id = s.id
		WHERE ps.playlists_id = $1
		ORDER BY position`,
		p.ID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var song Song
		var spotifyCheckedAt *time.Time
		var spotifyID *string

		err = rows.Scan(
			&song.ID,
			&song.Position,
			&song.Artist,
			&song.Title,
			&spotifyCheckedAt,
			&spotifyID,
		)
		if err != nil {
			return err
		}

		if spotifyCheckedAt != nil {
			song.SpotifyCheckedAt = *spotifyCheckedAt
		}

		if spotifyID != nil {
			song.SpotifyID = *spotifyID
		}

		p.Songs = append(p.Songs, &song)
	}

	return nil
}

// Song is an artist/title pair that we've extracted from a playlist.
type Song struct {
	// Artist is the name of the song's artist.
	Artist string

	// ID is the local database identifier of the song.
	ID int

	// Position is the track number of a song within a playlist.
	Position int

	// Title is the title of the song.
	Title string

	// SpotifyCheckedAt is the last time we tried to pull information on the
	// track from Spotify.
	SpotifyCheckedAt time.Time

	// SpotifyID is the canonical ID of the song according to Spotify.
	SpotifyID string
}

var outputDirs = []string{
	".",
	"assets",
	"assets/" + Release,
	"playlists",
}

// CreateOutputDirs creates a target directory for the static site and all
// other necessary directories for the build if they don't already exist.
func CreateOutputDirs(targetDir string) error {
	for _, dir := range outputDirs {
		dir = path.Join(targetDir, dir)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}

// RunTasks runs the given tasks in a pool.
//
// After the run, if any errors occurred, it prints the first 10. Returns true
// if all tasks succeeded. If a false is returned, the caller should consider
// exiting with non-zero status.
func RunTasks(concurrency int, tasks []*pool.Task) bool {
	log.Infof("Running %v task(s) with concurrency %v",
		len(tasks), concurrency)

	p := pool.NewPool(tasks, concurrency)
	p.Run()

	var numErrors int
	for _, task := range p.Tasks {
		if task.Err != nil {
			log.Errorf("Error: %v", task.Err.Error())
			numErrors++
		}
		if numErrors >= 10 {
			log.Errorf("Too many errors.")
			break
		}
	}
	return !p.HasErrors()
}
