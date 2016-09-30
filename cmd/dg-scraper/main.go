package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
)

const (
	// Base URL of the site with playlist index and playlist information. It's
	// sort of terrible to hardcode this, but we rate limit to make sure that
	// the site isn't hit very hard, and only need to retrieve any given
	// playlist one time.
	darkdbURL = "http://www.darkdb.com/deathguild"

	// The location of the playlist index.
	indexURL = darkdbURL + "/DateList.aspx"
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=2"`

	// DatabaseURL is a connection string for a database used to store playlist
	// and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`
}

// PlaylistLink is simply a URL to a playlist that we've pulled from the index.
type PlaylistLink string

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

	log.Printf("Requesting index at: %v", indexURL)
	resp, err := http.Get(indexURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	links, err := scrapeIndex(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var tasks []*pool.Task

	for _, link := range links {
		l := link
		tasks = append(tasks, pool.NewTask(func() error {
			return handlePlaylist(l)
		}))
	}

	log.Printf("Using goroutine pool with concurrency %v", conf.Concurrency)
	p := pool.NewPool(tasks, conf.Concurrency)
	p.Run()

	for _, task := range tasks {
		if task.Err != nil {
			log.Fatal(task.Err)
		}
	}
}

func scrapeIndex(r io.Reader) ([]PlaylistLink, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var outErr error
	var links []PlaylistLink

	doc.Find(".ListLink").EachWithBreak(func(i int, s *goquery.Selection) bool {
		link, ok := s.Attr("href")
		if !ok {
			outErr = fmt.Errorf("No href attribute for link: %v", s.Text())
			return false
		}

		links = append(links, PlaylistLink(link))
		log.Printf("Found: %v (%v)", link, s.Text())

		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	return links, nil
}

func handlePlaylist(link PlaylistLink) error {
	u, err := url.Parse(string(link))
	if err != nil {
		return err
	}
	day := u.Query().Get("date")

	txn, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		err = txn.Commit()
		if err != nil {
			panic(err)
		}
	}()

	var playlistID int
	err = txn.QueryRow(`
		SELECT id
		FROM playlists
		WHERE day = $1`,
		day,
	).Scan(&playlistID)

	if playlistID != 0 {
		log.Printf("Playlist %v already handled; skipping", day)
		return nil
	}

	log.Printf("Requesting playlist at: %v", darkdbURL+"/"+link)
	resp, err := http.Get(darkdbURL + "/" + string(link))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	songs, err := scrapePlaylist(resp.Body)
	if err != nil {
		return err
	}

	err = upsertPlaylistAndSongs(txn, day, songs)
	if err != nil {
		return err
	}

	// be kind and rate limit our requests
	t := 1 + rand.Float32()
	log.Printf("Sleeping %v seconds", t)
	time.Sleep(time.Duration(t) * time.Second)

	return nil
}

func upsertPlaylistAndSongs(txn *sql.Tx, day string,
	songs []*deathguild.Song) error {

	var playlistID int
	err := txn.QueryRow(`
		INSERT INTO playlists (day)
		VALUES ($1)
		ON CONFLICT (day) DO UPDATE
			SET day = excluded.day
		RETURNING id`,
		day,
	).Scan(&playlistID)
	if err != nil {
		return err
	}

	for i, song := range songs {
		var songID int
		err := txn.QueryRow(`
			INSERT INTO songs (artist, title)
			VALUES ($1, $2)
			ON CONFLICT (artist, title) DO UPDATE
				-- no-op
				SET artist = excluded.artist
			RETURNING id`,
			song.Artist,
			song.Title,
		).Scan(&songID)
		if err != nil {
			return err
		}

		_, err = txn.Exec(`
			INSERT INTO playlists_songs (playlists_id, songs_id, position)
			VALUES ($1, $2, $3)
			ON CONFLICT (playlists_id, songs_id) DO UPDATE
				SET position = excluded.position`,
			playlistID,
			songID,
			i,
		)
		if err != nil {
			return err
		}
	}

	log.Printf("Inserted records for %v song(s)", len(songs))
	return nil
}

func scrapePlaylist(r io.Reader) ([]*deathguild.Song, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var outErr error
	var songs []*deathguild.Song

	doc.Find("table.Normal tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		artist := s.Find("td:nth-child(1)").Text()
		title := s.Find("td:nth-child(2)").Text()

		// Ignore headers
		if artist == "Artist" && title == "Title" {
			return true
		}

		songs = append(songs, &deathguild.Song{Artist: artist, Title: title})
		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	log.Printf("Found playlist of %v song(s)", len(songs))
	return songs, nil
}
