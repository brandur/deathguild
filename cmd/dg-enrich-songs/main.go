package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/zmb3/spotify"
)

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
		// Do work in batches so we don't have to keep everything in memory
		// at once.
		songs, err := songsNeedingID(100)
		if err != nil {
			log.Fatal(err)
		}

		if len(songs) == 0 {
			log.Printf("Finished checking for song IDs")
			break
		}

		var tasks []*pool.Task
		var numNotFound int64

		for _, song := range songs {
			s := song
			tasks = append(tasks, pool.NewTask(func() error {
				return retrieveID(s, &numNotFound)
			}))
		}

		log.Printf("Using goroutine pool with concurrency %v",
			conf.Concurrency)
		p := pool.NewPool(tasks, conf.Concurrency)
		p.Run()

		log.Printf("Retrieved %v Spotify ID(s); failed to find %v",
			len(songs)-int(numNotFound), numNotFound)
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

func retrieveID(song *deathguild.Song, numNotFound *int64) error {
	searchString := fmt.Sprintf("artist:%v %v",
		song.Artist, song.Title)

	res, err := client.Search(searchString, spotify.SearchTypeTrack)
	if err != nil {
		return err
	}

	song.SpotifyCheckedAt = time.Now()

	if len(res.Tracks.Tracks) < 1 {
		log.Printf("Song not found: %+v", song)
		atomic.AddInt64(numNotFound, 1)

		err = updateSong(song)
		if err != nil {
			return err
		}

		return nil
	}

	track := res.Tracks.Tracks[0]

	log.Printf("Got track ID: %v (original: %v - %v) (Spotify: %v - %v)",
		string(track.ID),
		song.Artist, song.Title,
		artistsToString(track.Artists), track.Name)

	song.SpotifyID = string(track.ID)

	err = updateSong(song)
	if err != nil {
		return err
	}

	// be kind and rate limit our requests
	t := rand.Float32()
	log.Printf("Sleeping %v seconds", t)
	time.Sleep(time.Duration(t) * time.Second)

	return nil
}

func songsNeedingID(limit int) ([]*deathguild.Song, error) {
	rows, err := db.Query(`
		SELECT id, artist, title
		FROM songs
		WHERE spotify_id IS NULL
			AND (spotify_checked_at IS NULL
				-- periodically recheck Spotify for information that we failed
				-- to fill
				OR spotify_checked_at < NOW() - '1 month'::interval)
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

	log.Printf("Found %v songs needing Spotify IDs", len(songs))
	return songs, nil
}

func updateSong(song *deathguild.Song) error {
	// We want a NULL in this field with we didn't get an ID.
	var spotifyID *string
	if song.SpotifyID != "" {
		spotifyID = &song.SpotifyID
	}

	_, err := db.Exec(`
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