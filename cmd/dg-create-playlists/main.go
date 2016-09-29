package main

import (
	"database/sql"
	"log"

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
		playlists, err := playlistsNeedingID(100)
		if err != nil {
			log.Fatal(err)
		}

		if len(playlists) == 0 {
			log.Printf("Finished creating all playlists")
			break
		}

		var tasks []*pool.Task

		for _, playlist := range playlists {
			p := playlist
			tasks = append(tasks, pool.NewTask(func() error {
				return createPlaylist(p)
			}))
		}

		log.Printf("Using goroutine pool with concurrency %v",
			conf.Concurrency)
		p := pool.NewPool(tasks, conf.Concurrency)
		p.Run()

		log.Printf("Created %v Spotify playlist(s)", len(playlists))
	}
}

func createPlaylist(playlist *deathguild.Playlist) error {
	log.Printf("Created playlist %v with %v song(s)",
		playlist.FormattedDay(), len(playlist.Songs))
	return nil
}

func playlistsNeedingID(limit int) ([]*deathguild.Playlist, error) {
	rows, err := db.Query(`
		SELECT id, day
		FROM playlists
		WHERE spotify_id IS NULL
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []*deathguild.Playlist

	for rows.Next() {
		var playlist deathguild.Playlist
		err = rows.Scan(
			&playlist.ID,
			&playlist.Day,
		)
		if err != nil {
			return nil, err
		}
		playlists = append(playlists, &playlist)
	}

	for _, playlist := range playlists {
		rows, err := db.Query(`
			SELECT id, artist, title, spotify_checked_at, spotify_id
			FROM songs
			WHERE id IN (
					SELECT songs_id
					FROM playlists_songs
					WHERE playlists_id = $1
					ORDER BY position
				)
				AND spotify_id IS NOT NULL`,
			playlist.ID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var song deathguild.Song
			err = rows.Scan(
				&song.ID,
				&song.Artist,
				&song.Title,
				&song.SpotifyCheckedAt,
				&song.SpotifyID,
			)
			if err != nil {
				return nil, err
			}
			playlist.Songs = append(playlist.Songs, &song)
		}
	}

	log.Printf("Found %v playlist(s) needing Spotify IDs", len(playlists))
	return playlists, nil
}
