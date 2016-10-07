package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/assets"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/yosssi/ace"
)

// Conf contains configuration information for the command. It's extracted
// from environment variables.
type Conf struct {
	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=10"`

	// DatabaseURL is a connection string for a database used to store
	// playlist and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// LocalFonts starts using locally downloaded versions of Google Fonts.
	// This is not ideal for real deployment because you won't be able to
	// leverage Google's CDN and the caching that goes with it, and may not get
	// the font format for requesting browsers, but good for airplane rides
	// where you otherwise wouldn't have the fonts.
	LocalFonts bool `env:"LOCAL_FONTS,default=false"`

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

	versionedAssetsDir := path.Join(conf.TargetDir, "assets", deathguild.Release)

	txn, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := txn.Commit()
		if err != nil {
			panic(err)
		}
	}()

	playlistYears, err := loadPlaylistYears(txn)
	if err != nil {
		log.Fatal(err)
	}

	var tasks []*pool.Task

	tasks = append(tasks, pool.NewTask(func() error {
		return buildIndex(playlistYears)
	}))

	tasks = append(tasks, pool.NewTask(func() error {
		return linkFonts()
	}))

	tasks = append(tasks, pool.NewTask(func() error {
		return assets.CompileJavascripts(
			path.Join(".", "content", "javascripts"),
			path.Join(versionedAssetsDir, "app.js"))
	}))

	tasks = append(tasks, pool.NewTask(func() error {
		return assets.CompileStylesheets(
			path.Join(".", "content", "stylesheets"),
			path.Join(versionedAssetsDir, "app.css"))
	}))

	for _, playlistYear := range playlistYears {
		for _, playlist := range playlistYear.Playlists {
			p := playlist
			tasks = append(tasks, pool.NewTask(func() error {
				return buildPlaylist(p)
			}))
		}
	}

	if !deathguild.RunTasks(conf.Concurrency, tasks) {
		defer os.Exit(1)
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

func ensureSymlink(source, dest string) error {
	log.Debugf("Checking symbolic link (%v): %v -> %v",
		path.Base(source), source, dest)

	var actual string

	_, err := os.Stat(dest)

	// Note that if a symlink file does exist, but points to a non-existent
	// location, we still get an "does not exist" error back, so we fall down
	// to the general create path so that the symlink file can be removed.
	//
	// The call to RemoveAll does not affect the other path of the symlink file
	// not being present because it doesn't care whether or not the file it's
	// trying remove is actually there.
	if os.IsNotExist(err) {
		log.Debugf("Destination link does not exist. Creating.")
		goto create
	}
	if err != nil {
		return err
	}

	actual, err = os.Readlink(dest)
	if err != nil {
		return err
	}

	if actual == source {
		log.Debugf("Link exists.")
		return nil
	}

	log.Debugf("Destination links to wrong source. Creating.")

create:
	err = os.RemoveAll(dest)
	if err != nil {
		return err
	}

	return os.Symlink(source, dest)
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

func linkFonts() error {
	source, err := filepath.Abs(path.Join(".", "content", "fonts"))
	if err != nil {
		return err
	}

	dest, err := filepath.Abs(path.Join(conf.TargetDir, "assets", "fonts"))
	if err != nil {
		return err
	}

	return ensureSymlink(source, dest)
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

	return fmt.Sprintf("%v songs. %v songs (%.1f%%) were found in Spotify.",
		len(playlist.Songs), numWithSpotifyID, percent)
}

func renderTemplate(view, target string, locals map[string]interface{}) error {
	template, err := ace.Load("./layouts/main", view,
		&ace.Options{FuncMap: template.FuncMap{
			"PlaylistInfo":        playlistInfo,
			"SpotifyPlaylistLink": spotifyPlaylistLink,
			"SpotifySongLink":     spotifySongLink,
			"VerboseDate":         verboseDate,
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
		"LocalFonts": conf.LocalFonts,
		"Release":    deathguild.Release,
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

func spotifySongLink(song *deathguild.Song) string {
	return "https://open.spotify.com/track/" + song.SpotifyID
}

func verboseDate(t time.Time) string {
	return t.Format("January 2, 2006")
}
