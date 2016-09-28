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
	"github.com/brandur/sorg/pool"
	_ "github.com/lib/pq"
)

const concurrency = 2

const darkdbURL = "http://www.darkdb.com/deathguild"

const indexURL = darkdbURL + "/DateList.aspx"

type PlaylistLink string

type Song struct {
	Artist, Title string
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("postgres", "postgres://localhost/deathguild?sslmode=disable")
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

	log.Printf("Using goroutine pool with concurrency %v", concurrency)
	p := pool.NewPool(tasks, concurrency)
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

	var eventID int
	err = db.QueryRow(`
		SELECT id
		FROM events
		WHERE day = $1`,
		day,
	).Scan(&eventID)

	if eventID != 0 {
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

	err = upsertEventAndSongs(day, songs)
	if err != nil {
		return err
	}

	// be kind and rate limit our requests
	t := 1 + rand.Float32()
	log.Printf("Sleeping %v seconds", t)
	time.Sleep(time.Duration(t) * time.Second)

	return nil
}

func upsertEventAndSongs(day string, songs []*Song) error {
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	var eventID int
	err = txn.QueryRow(`
		INSERT INTO events (day)
		VALUES ($1)
		ON CONFLICT (day) DO UPDATE
			SET day = excluded.day
		RETURNING id`,
		day,
	).Scan(&eventID)
	if err != nil {
		return err
	}

	for _, song := range songs {
		var songID int
		err := txn.QueryRow(`
			INSERT INTO songs (artist, title)
			VALUES ($1, $2)
			ON CONFLICT (artist, title) DO UPDATE
				SET artist = excluded.artist
			RETURNING id`,
			song.Artist,
			song.Title,
		).Scan(&songID)
		if err != nil {
			return err
		}

		_, err = txn.Exec(`
			INSERT INTO events_songs (events_id, songs_id)
			VALUES ($1, $2)
			ON CONFLICT (events_id, songs_id) DO UPDATE
				SET events_id = excluded.events_id`,
			eventID,
			songID,
		)
		if err != nil {
			return err
		}
	}

	err = txn.Commit()
	if err != nil {
		return err
	}

	log.Printf("Inserted records for %v song(s)", len(songs))
	return nil
}

func scrapePlaylist(r io.Reader) ([]*Song, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var outErr error
	var songs []*Song

	doc.Find("table.Normal tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		artist := s.Find("td:nth-child(1)").Text()
		song := s.Find("td:nth-child(2)").Text()

		songs = append(songs, &Song{artist, song})

		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	return songs, nil
}
