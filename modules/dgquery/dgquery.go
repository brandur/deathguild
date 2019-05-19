package dgquery

import (
	"database/sql"

	"github.com/brandur/deathguild/modules/dgcommon"
	"github.com/lib/pq"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Public
//
//
//
//////////////////////////////////////////////////////////////////////////////

// PlaylistYear holds playlists grouped by year.
type PlaylistYear struct {
	Playlists []*dgcommon.Playlist
	Year      int
}

// PlaylistYears loads playlists and groups them by year.
func PlaylistYears(txn *sql.Tx) ([]*PlaylistYear, error) {
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

// ArtistRankingsByPlays loads artist rankings by total number of their songs
// played.
func ArtistRankingsByPlays(txn *sql.Tx, years []int, limit int) ([]*ArtistRanking, error) {
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

// ArtistRankingsBySongs loads artist rankings by the number of unique songs
// from each artist that were played.
func ArtistRankingsBySongs(txn *sql.Tx, years []int, limit int) ([]*ArtistRanking, error) {
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

// SongRankings loads songs by the number of plays.
func SongRankings(txn *sql.Tx, years []int, limit int) ([]*SongRanking, error) {
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
