package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/zmb3/spotify"
)

// Format for the names of Death Guild playlists.
const playlistNameFormat = "Death Guild Playlist - %v"

// Conf contains configuration information for the command. It's extracted
// from environment variables.
type Conf struct {
	// ClientID is our Spotify applicaton's client ID.
	ClientID string `env:"CLIENT_ID,required"`

	// ClientSecret is our Spotify applicaton's client secret.
	ClientSecret string `env:"CLIENT_SECRET,required"`

	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=5"`

	// DatabaseURL is a connection string for a database used to store
	// playlist and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// RefreshToken is our Spotify refresh token.
	RefreshToken string `env:"REFRESH_TOKEN,required"`
}

var client *spotify.Client
var conf Conf
var db *sql.DB
var userID string

func main() {
	var playlistMap map[string]spotify.ID
	var user *spotify.PrivateUser

	err := envdecode.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	txn, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	// Do one initial fetch out of the loop below just so that we can die very
	// early (and without having to wait for the Spotify playlist cache to
	// build) if we don't need to do any work.
	playlists, err := playlistsNeedingID(txn, 1)
	if err != nil {
		log.Fatal(err)
	}

	err = txn.Rollback()
	if err != nil {
		log.Fatal(err)
	}

	if len(playlists) == 0 {
		goto done
	}

	client = deathguild.GetSpotifyClient(
		conf.ClientID, conf.ClientSecret, conf.RefreshToken)

	// A user is needed for some API operations, so just cache one for the
	// whole set of requests.
	user, err = client.CurrentUser()
	if err != nil {
		log.Fatal(err)
	}
	userID = user.ID

	playlistMap, err = getPlaylistMap()
	if err != nil {
		log.Fatal(err)
	}

	for {
		txn, err := db.Begin()
		if err != nil {
			log.Fatal(err)
		}

		// Do work in batches so we don't have to keep everything in memory
		// at once.
		playlists, err := playlistsNeedingID(txn, 100)
		if err != nil {
			log.Fatal(err)
		}

		if len(playlists) == 0 {
			goto done
		}

		var tasks []*pool.Task

		for _, playlist := range playlists {
			p := playlist
			tasks = append(tasks, pool.NewTask(func() error {
				return createPlaylistWithSongs(txn, playlistMap, p)
			}))
		}

		log.Printf("Using goroutine pool with concurrency %v",
			conf.Concurrency)
		p := pool.NewPool(tasks, conf.Concurrency)
		p.Run()

		err = txn.Commit()
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Created %v Spotify playlist(s)", len(playlists))
	}

done:
	log.Printf("Finished creating all playlists")
}

func createPlaylist(name string) (spotify.ID, error) {
	playlist, err := client.CreatePlaylistForUser(userID, name, true)
	if err != nil {
		return spotify.ID(""), err
	}

	log.Printf(`Created playlist: "%v"`, name)
	return playlist.SimplePlaylist.ID, nil
}

func createPlaylistWithSongs(txn *sql.Tx, playlistMap map[string]spotify.ID,
	playlist *deathguild.Playlist) error {

	name := fmt.Sprintf(playlistNameFormat, playlist.FormattedDay())

	playlistID, ok := playlistMap[name]
	if !ok {
		var err error
		playlistID, err = createPlaylist(name)
		if err != nil {
			return err
		}
	} else {
		log.Printf(`Found cached playlist: "%v" (ID %v)`, name, playlistID)
	}

	var songIDs []spotify.ID
	for _, song := range playlist.Songs {
		songIDs = append(songIDs, spotify.ID(song.SpotifyID))
	}

	err := client.ReplacePlaylistTracks(userID, playlistID, songIDs...)
	if err != nil {
		return err
	}

	playlist.SpotifyID = string(playlistID)
	err = updatePlaylist(txn, playlist)
	if err != nil {
		return err
	}

	log.Printf(`Updated playlist: "%v" (ID %v) with %v song(s)`,
		name, playlistID, len(playlist.Songs))
	return nil
}

// Spotify has an incredibly low pagination limit, so it's much faster to just
// retrieve all playlists at once and run against them. This is obviously
// terrible because it guarantees race conditions, but it's better than a
// multi-hour runtime.
//
// Returns a map of playlist names mapped to IDs.
func getPlaylistMap() (map[string]spotify.ID, error) {
	playlistMap := make(map[string]spotify.ID)

	// Unfortunately 50 is as high as Spotify will go, meaning that our
	// pagination is pretty much guaranteed to get degradedly slow ...
	limit := 50
	offset := 0

	opts := &spotify.Options{
		Limit:  &limit,
		Offset: &offset,
	}

	for {
		page, err := client.CurrentUsersPlaylistsOpt(opts)
		if err != nil {
			return nil, err
		}

		// Reached the end of pagination.
		if len(page.Playlists) == 0 {
			break
		}

		for _, playlist := range page.Playlists {
			playlistMap[playlist.Name] = playlist.ID
		}

		offset += len(page.Playlists)
	}

	log.Printf("Cached %v playlist(s)", len(playlistMap))
	return playlistMap, nil
}

func playlistsNeedingID(txn *sql.Tx, limit int) ([]*deathguild.Playlist, error) {
	rows, err := txn.Query(`
		SELECT id, day
		FROM playlists
		WHERE spotify_id IS NULL
		-- create the most recent first
		ORDER BY day DESC
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
		rows, err := txn.Query(`
			SELECT id, artist, title, spotify_checked_at, spotify_id
			FROM songs
			WHERE id IN (
					SELECT songs_id
					FROM playlists_songs
					WHERE playlists_id = $1
					ORDER BY position
				)
				-- only select songs that have a known Spotify ID
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

func updatePlaylist(txn *sql.Tx, playlist *deathguild.Playlist) error {
	// We want a NULL in this field with we didn't get an ID.
	var spotifyID *string
	if playlist.SpotifyID != "" {
		spotifyID = &playlist.SpotifyID
	}

	_, err := txn.Exec(`
		UPDATE playlists
		SET spotify_id = $1
		WHERE id = $2`,
		spotifyID,
		playlist.ID,
	)
	return err
}
