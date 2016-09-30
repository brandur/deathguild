package deathguild

import (
	"log"
	"os"
	"path"
	"time"

	"github.com/brandur/sorg/pool"
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

var outputDirs = []string{
	".",
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

// Runs the given tasks in a pool.
//
// After the run, if any errors occurred, it prints the first 10. Returns true
// if all tasks succeeded. If a false is returned, the caller should consider
// exiting with non-zero status.
func RunTasks(concurrency int, tasks []*pool.Task) {
	log.Printf("Running %v task(s) with concurrency %v",
		len(tasks), concurrency)

	p := pool.NewPool(tasks, concurrency)
	p.Run()

	var numErrors int
	for _, task := range p.Tasks {
		if task.Err != nil {
			log.Printf("Error: %v", task.Err.Error())
			numErrors++
		}
		if numErrors >= 10 {
			log.Fatal("Too many errors.")
			break
		} else if p.HasErrors() {
			os.Exit(1)
		}
	}
}
