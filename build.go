package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/brandur/deathguild/modules/dgassets"
	"github.com/brandur/deathguild/modules/dgcommon"
	"github.com/brandur/modulir"
	"github.com/brandur/modulir/modules/mace"
	"github.com/brandur/modulir/modules/mfile"
	_ "github.com/lib/pq"
	"github.com/yosssi/ace"
	gocache "github.com/patrickmn/go-cache"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Constants
//
//
//
//////////////////////////////////////////////////////////////////////////////

const (
	layoutsMain = "./layouts/main.ace"
	viewsDir = "./views"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Build function
//
//
//
//////////////////////////////////////////////////////////////////////////////

func build(c *modulir.Context) []error {
	//
	// PHASE 0: Setup
	//
	// (No jobs should be enqueued here.)
	//

	c.Log.Debugf("Running build loop")

	versionedAssetsDir := path.Join(conf.TargetDir, "assets", dgcommon.Release)

	// Open database connection and transaction
	var err error
	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		return []error{err}
	}
	txn, err := db.Begin()
	if err != nil {
		return []error{err}
	}
	defer func() {
		if err := txn.Commit(); err != nil {
			panic(err)
		}
	}()

	// Generate a list of partial views.
	{
		partialViews = nil

		sources, err := readDirCached(c, c.SourceDir+"/views",
			&mfile.ReadDirOptions{ShowMeta: true})
		if err != nil {
			return []error{err}
		}

		for _, source := range sources {
			if strings.HasPrefix(filepath.Base(source), "_") {
				partialViews = append(partialViews, source)
			}
		}
	}

	//
	// PHASE 1
	//

	//
	// Common directories
	//
	// Create these outside of the job system because jobs below may depend on
	// their existence.
	//

	{
		commonDirs := []string{
			c.TargetDir + "/assets",
			c.TargetDir + "/playlists",
			versionedAssetsDir,
		}
		for _, dir := range commonDirs {
			err := mfile.EnsureDir(c, dir)
			if err != nil {
				return []error{nil}
			}
		}
	}

	//
	// Symlinks
	//

	{
		commonSymlinks := [][2]string{
			{c.SourceDir + "/content/fonts", c.TargetDir + "/assets/fonts"},
		}
		for _, link := range commonSymlinks {
			err := mfile.EnsureSymlink(c, link[0], link[1])
			if err != nil {
				return []error{nil}
			}
		}
	}

	playlistYears, err := loadPlaylistYears(txn)
	if err != nil {
		return []error{err}
	}

	//
	// Home
	//

	{
		c.AddJob("index", func() (bool, error) {
			return renderIndex(c, playlistYears)
		})
	}

	//
	// Javascripts
	//

	{
		c.AddJob("javascripts", func() (bool, error) {
			return compileJavascripts(c,
				c.SourceDir+"/content/javascripts",
				versionedAssetsDir+"/app.js")
		})
	}

	//
	// Playlists
	//


	for _, playlistYear := range playlistYears {
		for _, p := range playlistYear.Playlists {
			playlist := p

			name := fmt.Sprintf("playlist: %v", playlist.FormattedDay())
			c.AddJob(name, func() (bool, error) {
				return renderPlaylist(c, playlist)
			})
		}
	}

	//
	// Stylesheets
	//

	{
		c.AddJob("stylesheets", func() (bool, error) {
			return compileStylesheets(c,
				c.SourceDir+"/content/stylesheets",
				versionedAssetsDir+"/app.css")
		})
	}

	return nil
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Variables
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Connection to the database where we save and fetch information.
var db *sql.DB

// List of partial views. If any of these changes we rebuild pretty much
// everything. Even though some of those changes will false positives, the
// partials are used pervasively enough, and change infrequently enough, that
// it's worth the tradeoff. This variable is a global because so many render
// functions access it.
var partialViews []string

// An expiring cache that stores the results of a `mfile.ReadDir` (i.e. list
// directory) for some period of time. It turns out these calls are relatively
// slow and this helps speed up the build loop.
//
// Arguments are (defaultExpiration, cleanupInterval).
var readDirCache = gocache.New(5*time.Minute, 10*time.Minute)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Types
//
//
//
//////////////////////////////////////////////////////////////////////////////

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Variables
//
//
//
//////////////////////////////////////////////////////////////////////////////

// PlaylistYear holds playlists grouped by year.
type PlaylistYear struct {
	Playlists []*dgcommon.Playlist
	Year      int
}

func compileJavascripts(c *modulir.Context, sourceDir, target string) (bool, error) {
	sources, err := readDirCached(c, sourceDir, nil)
	if err != nil {
		return false, err
	}

	sourcesChanged := c.ChangedAny(sources...)
	if !sourcesChanged {
		return false, nil
	}

	return true, dgassets.CompileJavascripts(c, sourceDir, target)
}

func compileStylesheets(c *modulir.Context, sourceDir, target string) (bool, error) {
	sources, err := readDirCached(c, sourceDir, nil)
	if err != nil {
		return false, err
	}

	sourcesChanged := c.ChangedAny(sources...)
	if !sourcesChanged {
		return false, nil
	}

	return true, dgassets.CompileStylesheets(c, sourceDir, target)
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
		var playlist dgcommon.Playlist
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
func playlistInfo(playlist *dgcommon.Playlist) string {
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

func readDirCached(c *modulir.Context, source string,
	opts *mfile.ReadDirOptions) ([]string, error) {

	// Try to use a result from an expiring cache to speed up build loops that
	// run within close proximity of each other. Listing files is one of the
	// slower operations throughout the build loop, so this helps speed it up
	// quite a bit.
	//
	// Note that we only use the source as cache key even though technically
	// options could vary, which could potentially cause trouble. We know in
	// this project that ReadDir on particular directories always use the same
	// options, so we let that slide even if it's somewhat dangerous.
	if paths, ok := readDirCache.Get(source); ok {
		c.Log.Debugf("Using cached results of ReadDir: %s", source)
		return paths.([]string), nil
	}

	files, err := mfile.ReadDirWithOptions(c, source, opts)
	if err != nil {
		return nil, err
	}

	readDirCache.Set(source, files, gocache.DefaultExpiration)
	return files, nil
}

func renderIndex(c *modulir.Context, playlistYears []*PlaylistYear) (bool, error) {
	viewsChanged := c.ChangedAny(append(
		[]string{
			layoutsMain,
			viewsDir + "/index.ace",
		},
		partialViews...,
	)...)
	if !viewsChanged {
		return false, nil
	}

	err := renderTemplate(
		c,
		viewsDir + "/index.ace",
		c.TargetDir + "/index.html",
		map[string]interface{}{
			"PlaylistYears": playlistYears,
			"Title":         "Death Guild Spotify Playlists",
		},
	)
	return true, err
}

func renderPlaylist(c *modulir.Context, playlist *dgcommon.Playlist) (bool, error) {
	viewsChanged := c.ChangedAny(append(
		[]string{
			layoutsMain,
			viewsDir + "/playlist.ace",
		},
		partialViews...,
	)...)
	if !viewsChanged {
		return false, nil
	}

	txn, err := db.Begin()
	if err != nil {
		return true, err
	}

	err = renderPlaylistInTransaction(c, txn, playlist)
	if err != nil {
		return true, err
	}

	err = txn.Rollback()
	if err != nil {
		return true, err
	}

	return true, nil
}

func renderPlaylistInTransaction(c *modulir.Context, txn *sql.Tx,
		playlist *dgcommon.Playlist) error {

	err := playlist.FetchSongs(txn)
	if err != nil {
		return err
	}

	err = renderTemplate(
		c,
		viewsDir + "/playlist.ace",
		c.TargetDir + "/playlists/" + playlist.FormattedDay(),
		map[string]interface{}{
			"Playlist": playlist,
			"Title":    "Playlist for " + playlist.FormattedDay(),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func renderTemplate(c *modulir.Context, view, target string, locals map[string]interface{}) error {
	allLocals := map[string]interface{}{
		"DGEnv":             conf.DGEnv,
		"GoogleAnalyticsID": conf.GoogleAnalyticsID,
		"LocalFonts":        conf.LocalFonts,
		"Release":           dgcommon.Release,
	}

	// Override our basic data map with anything that the specific page sent
	// in.
	for k, v := range locals {
		allLocals[k] = v
	}

	err := mace.RenderFile(c, layoutsMain, view, target,
		&ace.Options{FuncMap: template.FuncMap{
			"PlaylistInfo":        playlistInfo,
			"SpotifyPlaylistLink": spotifyPlaylistLink,
			"SpotifySongLink":     spotifySongLink,
			"VerboseDate":         verboseDate,
		}}, allLocals)
	if err != nil {
		return err
	}

	return nil
}

func spotifyPlaylistLink(playlist *dgcommon.Playlist) string {
	return "https://open.spotify.com/user/" + conf.SpotifyUser +
		"/playlist/" + playlist.SpotifyID
}

func spotifySongLink(song *dgcommon.Song) string {
	return "https://open.spotify.com/track/" + song.SpotifyID
}

func verboseDate(t time.Time) string {
	return t.Format("January 2, 2006")
}
