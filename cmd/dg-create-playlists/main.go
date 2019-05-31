package main

import (
	"database/sql"
	"fmt"

	"github.com/brandur/deathguild/modules/dgcommon"
	"github.com/brandur/deathguild/modules/dgquery"
	"github.com/brandur/modulir"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/zmb3/spotify"
)

// Format for the names and descriptions of Death Guild playlists.
const (
	playlistAllTimeName        = "Death Guild — Top of all-time"
	playlistAllTimeDescription = `A compliation playlist of the top songs played at Death Guild for all time. See: https://deathguild.brandur.org/statistics`

	playlistDayNameFormat        = "Death Guild — %v"
	playlistDayDescriptionFormat = `A playlist played at the Death Guild event of %v. See: https://deathguild.brandur.org/playlists/%v.`

	playlistYearNameFormat        = "Death Guild — Top of %v"
	playlistYearDescriptionFormat = `A compliation playlist of the top songs played at Death Guild in %v. See: https://deathguild.brandur.org/statistics/%v.`
)

// Maximum number of playlists to try and handle in a single run.
const maxPlaylists = 200

// Concurrency level to run job pool at.
const poolConcurrency = 30

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
var log modulir.LoggerInterface = &modulir.Logger{Level: modulir.LevelInfo}
var playlistMap map[string]spotify.ID
var userID string

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	txn, err := db.Begin()
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	playlistYears, err := dgquery.PlaylistYears(txn)
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	err = txn.Rollback()
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	var pool *modulir.Pool

	pool = modulir.NewPool(log, poolConcurrency)
	defer pool.Stop()

	log.Infof("Starting work round")
	pool.StartRound()

	client = dgcommon.GetSpotifyClient(
		conf.ClientID, conf.ClientSecret, conf.RefreshToken)

	// A user is needed for some API operations, so just cache one for the
	// whole set of requests.
	userID, err = getCurrentUserID()
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	playlistMap, err = getPlaylistMap()
	if err != nil {
		dgcommon.ExitWithError(err)
	}

	// All-time playlists
	{
		allYears := make([]int, len(playlistYears))
		for i, year := range playlistYears {
			allYears[i] = year.Year
		}

		pool.Jobs <- modulir.NewJob("playlist: all-time", func() (bool, error) {
			return createPlaylistForYear(
				allYears,
				playlistAllTimeName,
				playlistAllTimeDescription,
			)
		})
	}

	// Per-year playlists
	{
		for _, y := range playlistYears {
			playlistYear := y

			name := fmt.Sprintf("playlist: %v", playlistYear.Year)
			pool.Jobs <- modulir.NewJob(name, func() (bool, error) {
				return createPlaylistForYear(
					[]int{playlistYear.Year},
					fmt.Sprintf(playlistYearNameFormat, playlistYear.Year),
					fmt.Sprintf(playlistYearDescriptionFormat, playlistYear.Year, playlistYear.Year),
				)
			})
		}
	}

	// Per-day playlists
	{
		playlists, err := getPlaylists(pool)
		if err != nil {
			dgcommon.ExitWithError(err)
		}

		for _, p := range playlists {
			playlist := p

			name := fmt.Sprintf("playlist: %v", playlist.FormattedDay())
			pool.Jobs <- modulir.NewJob(name, func() (bool, error) {
				return createPlaylistForDay(playlist)
			})
		}
	}

	pool.Wait()
	pool.LogErrors()
	pool.LogSlowest()

	if pool.JobsErrored != nil {
		dgcommon.ExitWithError(fmt.Errorf("%v job(s) errored occurred during last round",
			len(pool.JobsErrored)))
	}
}

func createPlaylist(name, description string) (spotify.ID, error) {
	playlist, err := client.CreatePlaylistForUser(userID, name, description, true)
	if err != nil {
		return spotify.ID(""), err
	}

	log.Debugf(`Created playlist: "%v"`, name)
	return playlist.SimplePlaylist.ID, nil
}

func createPlaylistForDay(playlist *dgcommon.Playlist) (bool, error) {
	txn, err := db.Begin()
	if err != nil {
		return false, err
	}

	name := fmt.Sprintf(playlistDayNameFormat, playlist.FormattedDay())
	description := fmt.Sprintf(playlistDayDescriptionFormat,
		playlist.FormattedDay(), playlist.FormattedDay())

	spotifyIDs := make([]spotify.ID, len(playlist.Songs))
	for i, song := range playlist.Songs {
		spotifyIDs[i] = spotify.ID(song.SpotifyID)
	}

	playlistID, err := createPlaylistWithSongs(txn, playlistMap, name, description, spotifyIDs)
	if err != nil {
		return true, err
	}

	playlist.SpotifyID = string(playlistID)
	err = updatePlaylist(txn, playlist)
	if err != nil {
		return true, err
	}

	err = txn.Commit()
	if err != nil {
		return true, err
	}

	return true, nil
}

func createPlaylistForYear(years []int, name, description string) (bool, error) {
	txn, err := db.Begin()
	if err != nil {
		return false, err
	}

	songRankings, err := dgquery.SongRankings(txn, years, 50, true)
	if err != nil {
		return false, err
	}

	spotifyIDs := make([]spotify.ID, len(songRankings))
	for i, ranking := range songRankings {
		spotifyIDs[i] = spotify.ID(ranking.SpotifyID)
	}

	playlistID, err := createPlaylistWithSongs(txn, playlistMap, name, description, spotifyIDs)
	if err != nil {
		return true, err
	}

	var slug string
	if len(years) == 1 {
		slug = fmt.Sprintf("%v", years[0])
	} else {
		slug = "all-time"
	}

	err = updatePlaylistSpecial(txn, slug, playlistID)
	if err != nil {
		return true, errors.Wrapf(err,
			"Error updating special playlist '%v' spotify ID", slug)
	}

	err = txn.Commit()
	if err != nil {
		return true, errors.Wrap(err, "Error committing transaction")
	}

	return true, nil
}

func createPlaylistWithSongs(txn *sql.Tx, playlistMap map[string]spotify.ID,
	name, description string, songIDs []spotify.ID) (spotify.ID, error) {

	playlistID, ok := playlistMap[name]
	if !ok {
		var err error
		playlistID, err = createPlaylist(name, description)
		if err != nil {
			return "", err
		}
	} else {
		log.Debugf(`Found cached playlist: "%v" (ID %v)`, name, playlistID)
	}

	// Spotify only allows us to add 100 tracks at once.
	//
	// Here we truncate down to 100, which is wrong, but to do anything else
	// would require some other non-idempotent playlist construction here (we
	// can no longer replace), so I'll have to rethink my strategy.
	if len(songIDs) > 100 {
		log.Errorf("Truncated playlist down to 100 songs for Spotify's benefit: %s", name)
		songIDs = songIDs[0:100]
	}

	err := client.ReplacePlaylistTracks(playlistID, songIDs...)
	if err != nil {
		return "", errors.Wrapf(err,
			"Error replacing tracks for playlist '%v' (spotify '%v')",
			name, playlistID)
	}

	log.Infof(`Updated playlist: "%v" (ID %v) with %v song(s)`,
		name, playlistID, len(songIDs))
	return playlistID, nil
}

func getCurrentUserID() (string, error) {
	user, err := client.CurrentUser()
	if err != nil {
		return "", err
	}
	return user.ID, nil
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

	log.Infof("Building playlist map")

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

	log.Infof("Cached %v playlist(s)", len(playlistMap))
	return playlistMap, nil
}

func getPlaylists(pool *modulir.Pool) ([]*dgcommon.Playlist, error) {
	txn, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		err := txn.Commit()
		if err != nil {
			panic(err)
		}
	}()

	// Do work in batches so we don't have to keep everything in memory
	// at once.
	return getPlaylistsInTransaction(txn, maxPlaylists)
}

func getPlaylistsInTransaction(txn *sql.Tx, limit int) ([]*dgcommon.Playlist, error) {
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

	var playlists []*dgcommon.Playlist

	for rows.Next() {
		var playlist dgcommon.Playlist
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
		err := playlist.FetchSongs(txn)
		if err != nil {
			return nil, err
		}
	}

	log.Infof("Found %v playlist(s) needing Spotify IDs", len(playlists))
	return playlists, nil
}

func updatePlaylist(txn *sql.Tx, playlist *dgcommon.Playlist) error {
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

func updatePlaylistSpecial(txn *sql.Tx, slug string, spotifyID spotify.ID) error {
	_, err := txn.Exec(`
		INSERT INTO special_playlists
			(spotify_id, slug)
		VALUES
			($1, $2)
		ON CONFLICT (slug)
			DO UPDATE SET spotify_id = EXCLUDED.spotify_id`,
		string(spotifyID),
		slug,
	)
	return err
}
