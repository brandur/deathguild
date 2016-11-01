package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/zmb3/spotify"
)

// The number of songs that we pull out of the database at a time and try to
// enrich. This essentially represents the size of the set that we'll
// atomically commit at once. We keep the number pretty small so that even if
// we encounter rate limiting from Spotify, we'll at least make some forward
// progress.
const batchSize = 20

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// ClientID is our Spotify applicaton's client ID.
	ClientID string `env:"CLIENT_ID,required"`

	// ClientSecret is our Spotify applicaton's client secret.
	ClientSecret string `env:"CLIENT_SECRET,required"`

	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=5"`

	// DatabaseURL is a connection string for a database used to store playlist
	// and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// Limit is an optional limit for the number of songs to try and enrich at
	// any given time. This is useful in CI because if there are too many
	// songs without IDs then the Spotify rate limits will fail the build
	// every time leaving it in a state of permanent failure. Instead, limit
	// to something that we know we can get under the limit and just slowly
	// collect all of the IDs over a series of subsequent builds that are run
	// over time.
	Limit int `env:"LIMIT,default=10000"`

	// RefreshToken is our Spotify refresh token.
	RefreshToken string `env:"REFRESH_TOKEN,required"`
}

var client *spotify.Client
var conf Conf
var db *sql.DB

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	client = deathguild.GetSpotifyClient(
		conf.ClientID, conf.ClientSecret, conf.RefreshToken)

	for {
		done, exitCode, err := runLoop()
		if err != nil {
			log.Fatal(err)
		}
		if done {
			defer os.Exit(exitCode)
			break
		}
	}
}

func artistsToString(artists []spotify.SimpleArtist) string {
	var out string
	for i, artist := range artists {
		if i != 0 {
			out += ", "
		}
		out += artist.Name
	}
	return out
}

func retrieveID(txn *sql.Tx, song *deathguild.Song, numNotFound *int64) error {
	song.SpotifyCheckedAt = time.Now()

	searchString := fmt.Sprintf("artist:%v %v",
		song.Artist, song.Title)
	res, err := client.Search(searchString, spotify.SearchTypeTrack)
	if err != nil {
		return err
	}

	if len(res.Tracks.Tracks) < 1 {
		searchString = fmt.Sprintf("artist:%v %v",
			song.Artist, song.Title)
		res, err = client.Search(searchString, spotify.SearchTypeTrack)
		if err != nil {
			return err
		}

		if len(res.Tracks.Tracks) < 1 {
			log.Debugf("Song not found: %+v", song)
			atomic.AddInt64(numNotFound, 1)

			err = updateSong(txn, song)
			if err != nil {
				return err
			}

			sleepWithJitter()
			return nil
		}
	}

	track := res.Tracks.Tracks[0]

	log.Debugf("Got track ID: %v (original: %v - %v) (Spotify: %v - %v)",
		string(track.ID),
		song.Artist, song.Title,
		artistsToString(track.Artists), track.Name)

	song.SpotifyID = string(track.ID)

	err = updateSong(txn, song)
	if err != nil {
		return err
	}

	sleepWithJitter()
	return nil
}

func runLoop() (bool, int, error) {
	txn, err := db.Begin()
	if err != nil {
		return false, 0, err
	}
	defer func() {
		err := txn.Commit()
		if err != nil {
			panic(err)
		}
	}()

	// Do work in batches so we don't have to keep everything in memory
	// at once.
	songs, err := songsNeedingID(txn, batchSize)
	if err != nil {
		return false, 0, err
	}

	if len(songs) == 0 {
		return true, 0, nil
	}

	var tasks []*pool.Task
	var numNotFound int64

	for _, song := range songs {
		s := song
		tasks = append(tasks, pool.NewTask(func() error {
			return retrieveID(txn, s, &numNotFound)
		}))
	}

	if !deathguild.RunTasks(conf.Concurrency, tasks) {
		return true, 1, nil
	}

	log.Infof("Retrieved %v Spotify ID(s); failed to find %v",
		len(songs)-int(numNotFound), numNotFound)

	if len(songs) >= conf.Limit {
		log.Infof("Hit configured song limit of %v; dying peacefully",
			len(songs))
		return true, 0, nil
	}

	return false, 0, nil
}

// Spotify has an extremely draconian rate limit even for registered apps
// so sleep between requests.
func sleepWithJitter() {
	// This is range of 1 to 2 seconds. This seems to be experimentally
	// adequate at concurrency one to generally keep us under limit.
	t := rand.Float32() + 1
	log.Infof("Sleeping %v seconds", t)
	time.Sleep(time.Duration(t) * time.Second)
}

func songsNeedingID(txn *sql.Tx, limit int) ([]*deathguild.Song, error) {
	rows, err := txn.Query(`
		SELECT id, artist, title
		FROM songs
		WHERE spotify_id IS NULL
			AND (spotify_checked_at IS NULL
				-- Periodically recheck Spotify for information that we failed
				-- to fill.
				--
				-- We jitter from the last check time by a random interval
				-- between 0 seconds and a week just so that we aren't trying to
				-- do huge batches with similar check times all at the same time.
				-- (Basically there was a single massive batch from back when I
				-- did the initial backfill).
				OR spotify_checked_at + (random() * '1 week'::interval) <
					NOW() - '1 months'::interval)

		-- Prefer newer songs because it's more likely that we'll successfully
		-- find IDs for them. The older stuff tends to be songs that will probably
		-- never get an ID but have re-entered a check window.
		ORDER BY id DESC

		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []*deathguild.Song

	for rows.Next() {
		var song deathguild.Song
		err = rows.Scan(
			&song.ID,
			&song.Artist,
			&song.Title,
		)
		if err != nil {
			return nil, err
		}
		songs = append(songs, &song)
	}

	log.Infof("Found %v songs needing Spotify IDs", len(songs))
	return songs, nil
}

func updateSong(txn *sql.Tx, song *deathguild.Song) error {
	// We want a NULL in this field with we didn't get an ID.
	var spotifyID *string
	if song.SpotifyID != "" {
		spotifyID = &song.SpotifyID
	}

	_, err := txn.Exec(`
		UPDATE songs
		SET spotify_checked_at = $1,
			spotify_id = $2
		WHERE id = $3`,
		song.SpotifyCheckedAt,
		spotifyID,
		song.ID,
	)
	return err
}
