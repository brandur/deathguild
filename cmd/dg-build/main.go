package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"os"
	"path"

	//"github.com/brandur/sorg/pool"
	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/yosssi/ace"
)

const (
	// Release allows CSS and JS assets to be invalidated quickly by changing
	// their URL. Bump this number whenever something significant changes that
	// should be invalidated as quickly as possible.
	Release = 1
)

// Conf contains configuration information for the command. It's extracted
// from environment variables.
type Conf struct {
	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=30"`

	// DatabaseURL is a connection string for a database used to store
	// playlist and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// SpotifyUser is the name of the Spotify user who owns the Death Guild
	// playlists. This is used to generate links.
	SpotifyUser string `env:"SPOTIFY_USER,default=fyrerise"`

	// TargetDir is the target location where the site will be built to.
	TargetDir string `env:"TARGET_DIR,default=./public"`
}

// PlaylistYear holds playlists grouped by year.
type PlaylistYear struct {
	Playlists []*deathguild.Playlist
	Year      int
}

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

	err = deathguild.CreateOutputDirs(conf.TargetDir)
	if err != nil {
		log.Fatal(err)
	}

	txn, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	playlistYears, err := loadPlaylistYears(txn)
	if err != nil {
		log.Fatal(err)
	}

	var tasks []*pool.Task

	tasks = append(tasks, pool.NewTask(func() error {
		return buildIndex(playlistYears)
	}))

	for _, playlistYear := range playlistYears {
		for _, playlist := range playlistYear.Playlists {
			p := playlist
			tasks = append(tasks, pool.NewTask(func() error {
				return buildPlaylist(p)
			}))
		}
	}

	deathguild.RunTasks(conf.Concurrency, tasks)

	err = txn.Rollback()
	if err != nil {
		log.Fatal(err)
	}
}

func buildIndex(playlistYears []*PlaylistYear) error {
	err := renderTemplate(
		path.Join(".", "views", "index"),
		path.Join(conf.TargetDir, "index.html"),
		map[string]interface{}{
			"PlaylistYears": playlistYears,
			"Title":         "Playlists Index",
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func buildPlaylist(playlist *deathguild.Playlist) error {
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	err = buildPlaylistInTransaction(txn, playlist)
	if err != nil {
		return err
	}

	err = txn.Rollback()
	if err != nil {
		return err
	}

	return nil
}

func buildPlaylistInTransaction(txn *sql.Tx, playlist *deathguild.Playlist) error {
	err := playlist.FetchSongs(txn)
	if err != nil {
		return err
	}

	err = renderTemplate(
		path.Join(".", "views", "playlist"),
		path.Join(conf.TargetDir, "playlists", playlist.FormattedDay()),
		map[string]interface{}{
			"Playlist": playlist,
			"Title":    "Death Guild - " + playlist.FormattedDay(),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func loadPlaylistYears(txn *sql.Tx) ([]*PlaylistYear, error) {
	rows, err := txn.Query(`
		SELECT id, day, spotify_id
		FROM playlists
		WHERE spotify_id IS NOT NULL
		-- create the most recent first
		ORDER BY day DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlistYear *PlaylistYear
	var playlistYears []*PlaylistYear

	for rows.Next() {
		var playlist deathguild.Playlist
		err = rows.Scan(
			&playlist.ID,
			&playlist.Day,
			&playlist.SpotifyID,
		)
		if err != nil {
			return nil, err
		}

		if playlistYear == nil || playlistYear.Year != playlist.Day.Year() {
			playlistYear = &PlaylistYear{Year: playlist.Day.Year()}
			playlistYears = append(playlistYears, playlistYear)
		}

		playlistYear.Playlists = append(playlistYear.Playlists, &playlist)
	}

	return playlistYears, nil
}

// Returns some basic length information about the playlist.
func playlistInfo(playlist *deathguild.Playlist) string {
	var numWithSpotifyID int
	for _, song := range playlist.Songs {
		if song.SpotifyID != "" {
			numWithSpotifyID++
		}
	}

	percent := float64(numWithSpotifyID) / float64(len(playlist.Songs)) * 100

	return fmt.Sprintf("%v song(s). %v song(s) (%.1f%%) found in Spotify.",
		len(playlist.Songs), numWithSpotifyID, percent)
}

func renderTemplate(view, target string, locals map[string]interface{}) error {
	template, err := ace.Load("./layouts/main", view,
		&ace.Options{FuncMap: template.FuncMap{
			"PlaylistInfo":        playlistInfo,
			"SpotifyPlaylistLink": spotifyPlaylistLink,
		}})
	if err != nil {
		return err
	}

	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	data := map[string]interface{}{
		"Release": Release,
	}

	// Override our basic data map with anything that the specific page sent
	// in.
	for k, v := range locals {
		data[k] = v
	}

	err = template.Execute(writer, data)
	if err != nil {
		return err
	}

	return nil
}

func spotifyPlaylistLink(playlist *deathguild.Playlist) string {
	return "https://open.spotify.com/user/" + conf.SpotifyUser +
		"/playlist/" + playlist.SpotifyID
}
