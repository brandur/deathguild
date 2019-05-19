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
	"github.com/lib/pq"
	gocache "github.com/patrickmn/go-cache"
	"github.com/yosssi/ace"
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
	viewsDir    = "./views"
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
	var db *sql.DB
	var txn *sql.Tx
	{
		var err error

		db, err = sql.Open("postgres", conf.DatabaseURL)
		if err != nil {
			return []error{err}
		}

		txn, err = db.Begin()
		if err != nil {
			return []error{err}
		}
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
			c.TargetDir + "/statistics",
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

	for _, y := range playlistYears {
		for _, p := range y.Playlists {
			playlist := p

			name := fmt.Sprintf("playlist: %v", playlist.FormattedDay())
			c.AddJob(name, func() (bool, error) {
				return renderPlaylist(c, db, playlist)
			})
		}
	}

	//
	// Playlists
	//

	for _, y := range playlistYears {
		playlistYear := y

		name := fmt.Sprintf("statistics: %v", playlistYear.Year)
		c.AddJob(name, func() (bool, error) {
			return renderStatisticsYear(c, db, playlistYear)
		})
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
// Build functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

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
		viewsDir+"/index.ace",
		c.TargetDir+"/index.html",
		viewsChanged,
		map[string]interface{}{
			"PlaylistYears": playlistYears,
			"Title":         "Death Guild Spotify Playlists",
		},
	)
	return true, err
}

func renderPlaylist(c *modulir.Context, db *sql.DB, playlist *dgcommon.Playlist) (bool, error) {
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

	err = renderPlaylistInTransaction(c, txn, viewsChanged, playlist)
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
	viewsChanged bool, playlist *dgcommon.Playlist) error {

	err := playlist.FetchSongs(txn)
	if err != nil {
		return err
	}

	err = renderTemplate(
		c,
		viewsDir+"/playlist.ace",
		c.TargetDir+"/playlists/"+playlist.FormattedDay(),
		viewsChanged,
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

func renderStatisticsYear(c *modulir.Context, db *sql.DB, playlistYear *PlaylistYear) (bool, error) {
	viewsChanged := c.ChangedAny(append(
		[]string{
			layoutsMain,
			viewsDir + "/statistics/year.ace",
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

	err = renderStatisticsYearInTransaction(c, txn, viewsChanged, playlistYear)
	if err != nil {
		return true, err
	}

	err = txn.Rollback()
	if err != nil {
		return true, err
	}

	return true, nil
}

func renderStatisticsYearInTransaction(c *modulir.Context, txn *sql.Tx,
	viewsChanged bool, playlistYear *PlaylistYear) error {

	artistRankingsByPlays, err := loadArtistRankingsByPlays(
		txn, []int{playlistYear.Year}, 15)
	if err != nil {
		return err
	}

	artistRankingsBySongs, err := loadArtistRankingsBySongs(
		txn, []int{playlistYear.Year}, 15)
	if err != nil {
		return err
	}

	songRankings, err := loadSongRankings(
		txn, []int{playlistYear.Year}, 15)
	if err != nil {
		return err
	}

	err = renderTemplate(
		c,
		viewsDir+"/statistics/year.ace",
		c.TargetDir+"/statistics/"+fmt.Sprintf("%v", playlistYear.Year),
		viewsChanged,
		map[string]interface{}{
			"ArtistRankingsByPlays": artistRankingsByPlays,
			"ArtistRankingsBySongs": artistRankingsBySongs,
			"SongRankings":          songRankings,
			"Title":                 fmt.Sprintf("Statistics for %v", playlistYear.Year),
			"Year":                  playlistYear.Year,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Query functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

// PlaylistYear holds playlists grouped by year.
type PlaylistYear struct {
	Playlists []*dgcommon.Playlist
	Year      int
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

// ArtistRanking is a record that ranks an artist by plays.
type ArtistRanking struct {
	Artist string
	Count  int
}

// Loads artist rankings by total number of their songs played.
func loadArtistRankingsByPlays(txn *sql.Tx, years []int, limit int) ([]*ArtistRanking, error) {
	rows, err := txn.Query(`
		WITH year_songs AS (
			SELECT artist, title, s.spotify_id AS song_spotify_id
			FROM playlists p
				INNER JOIN playlists_songs ps
					ON p.id = ps.playlists_id
				INNER JOIN songs s
					ON s.id = ps.songs_id
			WHERE date_part('year', p.day) = any($1)
		)
		SELECT artist, count(*)
		FROM year_songs
		GROUP BY artist
		ORDER BY count DESC
		LIMIT $2`,
		pq.Array(years),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []*ArtistRanking

	for rows.Next() {
		var ranking ArtistRanking
		err = rows.Scan(
			&ranking.Artist,
			&ranking.Count,
		)
		if err != nil {
			return nil, err
		}

		rankings = append(rankings, &ranking)
	}

	return rankings, nil
}

// Loads artist rankings by the number of unique songs from each artist that
// were played.
func loadArtistRankingsBySongs(txn *sql.Tx, years []int, limit int) ([]*ArtistRanking, error) {
	rows, err := txn.Query(`
		WITH year_songs AS (
			SELECT artist, title, s.spotify_id AS song_spotify_id
			FROM playlists p
				INNER JOIN playlists_songs ps
					ON p.id = ps.playlists_id
				INNER JOIN songs s
					ON s.id = ps.songs_id
			WHERE date_part('year', p.day) = any($1)
		)
		SELECT artist, count(distinct(title))
		FROM year_songs
		GROUP BY artist
		ORDER BY count DESC
		LIMIT $2`,
		pq.Array(years),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []*ArtistRanking

	for rows.Next() {
		var ranking ArtistRanking
		err = rows.Scan(
			&ranking.Artist,
			&ranking.Count,
		)
		if err != nil {
			return nil, err
		}

		rankings = append(rankings, &ranking)
	}

	return rankings, nil
}

// SongRanking is a record that ranks an artist by plays.
type SongRanking struct {
	Artist    string
	Title     string
	SpotifyID string
	Count     int
}

// Loads songs by the number of plays.
func loadSongRankings(txn *sql.Tx, years []int, limit int) ([]*SongRanking, error) {
	rows, err := txn.Query(`
		WITH year_songs AS (
			SELECT artist, title, s.spotify_id AS song_spotify_id
			FROM playlists p
				INNER JOIN playlists_songs ps
					ON p.id = ps.playlists_id
				INNER JOIN songs s
					ON s.id = ps.songs_id
			WHERE date_part('year', p.day) = any($1)
		)
		SELECT artist, title, song_spotify_id, count(*)
		FROM year_songs
		GROUP BY artist, title, song_spotify_id
		ORDER BY count DESC
		LIMIT $2`,
		pq.Array(years),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []*SongRanking

	for rows.Next() {
		var ranking SongRanking
		var spotifyID *string

		err = rows.Scan(
			&ranking.Artist,
			&ranking.Title,
			&spotifyID,
			&ranking.Count,
		)
		if err != nil {
			return nil, err
		}

		if spotifyID != nil {
			ranking.SpotifyID = *spotifyID
		}

		rankings = append(rankings, &ranking)
	}

	return rankings, nil
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Template functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

var templateFuncMap = template.FuncMap{
	"Add":                 add,
	"PlaylistInfo":        playlistInfo,
	"SpotifyPlaylistLink": spotifyPlaylistLink,
	"SpotifySongLink":     spotifySongLink,
	"VerboseDate":         verboseDate,
}

// Performs basic arithmetic because Go templates don't allow for this in any
// other way.
func add(x, y int) int {
	return x + y
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

func spotifyPlaylistLink(playlist *dgcommon.Playlist) string {
	return "https://open.spotify.com/user/" + conf.SpotifyUser +
		"/playlist/" + playlist.SpotifyID
}

func spotifySongLink(spotifyID string) string {
	return "https://open.spotify.com/track/" + spotifyID
}

func verboseDate(t time.Time) string {
	return t.Format("January 2, 2006")
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Other functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

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

func renderTemplate(c *modulir.Context, view, target string, dynamicReload bool,
	locals map[string]interface{}) error {

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

	options := &ace.Options{FuncMap: templateFuncMap}
	if dynamicReload {
		options.DynamicReload = true
	}

	err := mace.RenderFile(c, layoutsMain, view, target, options, allLocals)

	if err != nil {
		return err
	}

	return nil
}
