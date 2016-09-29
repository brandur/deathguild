package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/brandur/deathguild"
	"github.com/joeshaw/envdecode"
	"github.com/zmb3/spotify"
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// ClientID is our Spotify applicaton's client ID.
	ClientID string `env:"CLIENT_ID,required"`

	// ClientSecret is our Spotify applicaton's client secret.
	ClientSecret string `env:"CLIENT_SECRET,required"`

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

	songs, err := songsNeedingID()
	if err != nil {
		log.Fatal(err)
	}

	err = retrieveIDs(songs)
	if err != nil {
		log.Fatal(err)
	}
}

func retrieveIDs(songs []*deathguild.Song) error {
	for _, song := range songs {
		searchString := fmt.Sprintf("artist:%v title:%v",
			song.Artist, song.Title)

		res, err := client.Search(searchString, spotify.SearchTypeArtist)
		if err != nil {
			return nil
		}

		if len(res.Tracks.Tracks) < 1 {
			log.Printf("Song not found: %+v", song)
			continue
		}

		track := res.Tracks.Tracks[0]
		song.SpotifyID = string(track.ID)
	}

	log.Printf("Retrieved %v Spotify ID(s) for songs", len(songs))

	return nil
}

func songsNeedingID() ([]*deathguild.Song, error) {
	rows, err := db.Query(`
		SELECT artist, title
		FROM songs
		WHERE spotify_id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []*deathguild.Song

	for rows.Next() {
		var song deathguild.Song
		err = rows.Scan(
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
