package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/brandur/deathguild"
	"github.com/brandur/sorg/pool"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/yosssi/ace"
	"github.com/yosssi/gcss"
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
		return compileJavascripts(path.Join(".", "javascripts"),
			path.Join(versionedAssetsDir, "app.js"))
	}))

	tasks = append(tasks, pool.NewTask(func() error {
		return compileStylesheets(path.Join(".", "stylesheets"),
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
		os.Exit(1)
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

// Compiles a set of JavaScript files into a single large file by appending
// them all to each other. Files are appended in alphabetical order so we
// depend on the fact that there aren't too many interdependencies between
// files. A common requirement can be given an underscore prefix to be loaded
// first.
//
// Note that this function was copied wholesale from sorg and should probably
// be shared between projects.
func compileJavascripts(inPath, outPath string) error {
	log.Printf("Building: %v", outPath)

	javascriptInfos, err := ioutil.ReadDir(inPath)
	if err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, javascriptInfo := range javascriptInfos {
		if isHidden(javascriptInfo.Name()) {
			continue
		}

		log.Printf("Including: %v", javascriptInfo.Name())

		inFile, err := os.Open(path.Join(inPath, javascriptInfo.Name()))
		if err != nil {
			return err
		}

		outFile.WriteString("/* " + javascriptInfo.Name() + " */\n\n")
		outFile.WriteString("(function() {\n\n")

		_, err = io.Copy(outFile, inFile)
		if err != nil {
			return err
		}

		outFile.WriteString("\n\n")
		outFile.WriteString("}).call(this);\n\n")
	}

	return nil
}

// Compiles a set of stylesheet files into a single large file by appending
// them all to each other. Files are appended in alphabetical order so we
// depend on the fact that there aren't too many interdependencies between
// files. CSS reset in particular is given an underscore prefix so that it gets
// to load first.
//
// If a file has a ".sass" suffix, we attempt to render it as GCSS. This isn't
// a perfect symmetry, but works well enough for these cases.
//
// Note that this function was copied wholesale from sorg and should probably
// be shared between projects.
func compileStylesheets(inPath, outPath string) error {
	log.Printf("Building: %v", outPath)

	stylesheetInfos, err := ioutil.ReadDir(inPath)
	if err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, stylesheetInfo := range stylesheetInfos {
		if isHidden(stylesheetInfo.Name()) {
			continue
		}

		log.Printf("Including: %v", stylesheetInfo.Name())

		inFile, err := os.Open(path.Join(inPath, stylesheetInfo.Name()))
		if err != nil {
			return err
		}

		outFile.WriteString("/* " + stylesheetInfo.Name() + " */\n\n")

		if strings.HasSuffix(stylesheetInfo.Name(), ".sass") {
			_, err := gcss.Compile(outFile, inFile)
			if err != nil {
				return fmt.Errorf("Error compiling %v: %v",
					stylesheetInfo.Name(), err)
			}
		} else {
			_, err := io.Copy(outFile, inFile)
			if err != nil {
				return err
			}
		}

		outFile.WriteString("\n\n")
	}

	return nil
}

// Detects a hidden file, i.e. one that starts with a dot.
//
// Note that this function was copied wholesale from sorg and should probably
// be shared between projects.
func isHidden(file string) bool {
	return strings.HasPrefix(file, ".")
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

	return fmt.Sprintf("%v songs. %v songs (%.1f%%) found in Spotify.",
		len(playlist.Songs), numWithSpotifyID, percent)
}

func renderTemplate(view, target string, locals map[string]interface{}) error {
	template, err := ace.Load("./layouts/main", view,
		&ace.Options{FuncMap: template.FuncMap{
			"PlaylistInfo":        playlistInfo,
			"SpotifyPlaylistLink": spotifyPlaylistLink,
			"SpotifySongLink":     spotifySongLink,
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
		"Release": deathguild.Release,
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
