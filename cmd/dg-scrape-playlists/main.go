package main

import (
	"database/sql"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/brandur/deathguild/modules/dgcommon"
	"github.com/brandur/modulir"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
)

const (
	// Base URL of the site with playlist index and playlist information. It's
	// sort of terrible to hardcode this, but we rate limit to make sure that
	// the site isn't hit very hard, and only need to retrieve any given
	// playlist one time.
	baseURL = "http://www.deathguild.com"

	// The location of the playlist index.
	indexURL = baseURL + "/playdates"

	// Concurrency level to run job pool at.
	poolConcurrency = 20
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
var log modulir.LoggerInterface = &modulir.Logger{Level: modulir.LevelInfo}

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	log.Infof("Requesting index at: %v", indexURL)
	resp, err := http.Get(indexURL)
	if err != nil {
		dgcommon.ExitWithError(err)
	}
	defer resp.Body.Close()

	links, err := scrapeIndex(resp.Body)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	pool := modulir.NewPool(log, poolConcurrency)
	defer pool.Stop()

	for _, l := range links {
		link := l

		name := fmt.Sprintf("playlist: %v", link)
		pool.Jobs <- modulir.NewJob(name, func() (bool, error) {
			return handlePlaylist(link)
		})
	}

	pool.Wait()
	pool.LogErrors()
	pool.LogSlowest()

	if pool.JobsErrored != nil {
		dgcommon.ExitWithError(fmt.Errorf("%v job(s) errored occurred during last loop",
			len(pool.JobsErrored)))
	}
}

func scrapeIndex(r io.Reader) ([]PlaylistLink, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var outErr error
	var links []PlaylistLink

	doc.Find("#playlist table a").EachWithBreak(func(i int, s *goquery.Selection) bool {
		link, ok := s.Attr("href")
		if !ok {
			outErr = fmt.Errorf("No href attribute for link: %v", s.Text())
			return false
		}

		links = append(links, PlaylistLink(link))

		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	log.Infof("Found %v playlist(s) in index", len(links))
	return links, nil
}

func extractDay(link PlaylistLink) string {
	parts := strings.Split(string(link), "/")
	return parts[len(parts)-1]
}

func handlePlaylist(link PlaylistLink) (bool, error) {
	var retErr error
	day := extractDay(link)

	txn, err := db.Begin()
	if err != nil {
		retErr = err
		return false, retErr
	}
	defer func() {
		// Don't try to commit with an error because we'll end up overriding
		// the more useful error message. Just fall through.
		if retErr != nil {
			return
		}

		err = txn.Commit()
		if err != nil {
			retErr = err
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
		log.Infof("Playlist %v already handled; skipping", day)
		retErr = nil
		return true, retErr
	}

	playlistURL := string(baseURL + link)
	log.Debugf("Requesting playlist at: %v", playlistURL)
	resp, err := http.Get(playlistURL)
	if err != nil {
		retErr = err
		return true, retErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retErr = fmt.Errorf("bad response when requesting: %s (status code = %v)",
			playlistURL, resp.StatusCode)
		return true, retErr
	}

	songs, err := scrapePlaylist(resp.Body)
	if err != nil {
		retErr = err
		return true, retErr
	}

	// If we got a playlist of zero songs, that probably means our DOM/HTML
	// selectors aren't working anymore because the site has changed its
	// format. Error and tell somebody.
	if len(songs) == 0 {
		retErr = fmt.Errorf(
			"found zero-length playlist; this probably means that scraping logic is broken",
		)
		return true, retErr
	}

	err = upsertPlaylistAndSongs(txn, day, songs)
	if err != nil {
		retErr = err
		return true, retErr
	}

	// be kind and rate limit our requests
	t := 1 + rand.Float32()
	log.Debugf("Sleeping %v seconds", t)
	time.Sleep(time.Duration(t) * time.Second)

	retErr = nil
	return true, nil
}

func upsertPlaylistAndSongs(txn *sql.Tx, day string,
	songs []*dgcommon.Song) error {

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
		return fmt.Errorf("Error inserting into `playlists`: %v", err)
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
			return fmt.Errorf("Error inserting into `songs`: %v", err)
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
			return fmt.Errorf("Error inserting into `xplaylist_songs`: %v", err)
		}
	}

	log.Infof("Inserted records for %v song(s)", len(songs))
	return nil
}

func scrapePlaylist(r io.Reader) ([]*dgcommon.Song, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var outErr error
	var songs []*dgcommon.Song

	// Old style playlists
	doc.Find("table.Normal tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		artist := s.Find("td:nth-child(1)").Text()
		title := s.Find("td:nth-child(2)").Text()

		// Ignore headers
		if artist == "Artist" && title == "Title" {
			return true
		}

		songs = append(songs, &dgcommon.Song{Artist: artist, Title: title})
		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	// New style playlists
	doc.Find("div#playlist em").EachWithBreak(func(i int, s *goquery.Selection) bool {
		artist := s.Text()

		allContent, err := s.Parent().Html()
		if err != nil {
			outErr = err
			return false
		}

		// Unfortunately we're trying to parse Very Bad HTML which carries no
		// semantic data whatsoever, so rather than traverse the DOM, we need
		// to depend on a regexp hack to get titles out of the document.
		//
		// Be careful to:
		//
		//     1. Escape any HTML specific characters in the string (the call
		//        to `Text` above will have unescaped them).
		//     2. Escape any characters that are meaningful to a regexp (e.g., `+`).
		rx := regexp.MustCompile(fmt.Sprintf(`<em>%s</em> - (.*?)<`,
			regexp.QuoteMeta(html.EscapeString(artist))))

		matches := rx.FindStringSubmatch(allContent)

		if len(matches) < 2 {
			outErr = fmt.Errorf("Failed to find title match for: %s", artist)
			return false
		}

		// First index is the entire match, the second is our capture group.
		title := html.UnescapeString(matches[1])

		songs = append(songs, &dgcommon.Song{Artist: artist, Title: title})
		return true
	})
	if outErr != nil {
		return nil, outErr
	}

	log.Infof("Found playlist of %v song(s)", len(songs))
	return songs, nil
}
