// Copyright 2014, 2015 Zac Bergquist
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spotify

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SimpleTrack contains basic info about a track.
type SimpleTrack struct {
	Artists []SimpleArtist `json:"artists"`
	// A list of the countries in which the track can be played,
	// identified by their ISO 3166-1 alpha-2 codes.
	AvailableMarkets []string `json:"available_markets"`
	// The disc number (usually 1 unless the album consists of more than one disc).
	DiscNumber int `json:"disc_number"`
	// The length of the track, in milliseconds.
	Duration int `json:"duration_ms"`
	// Whether or not the track has explicit lyrics.
	// true => yes, it does; false => no, it does not.
	Explicit bool `json:"explicit"`
	// External URLs for this track.
	ExternalURLs ExternalURL `json:"external_urls"`
	// A link to the Web API endpoint providing full details for this track.
	Endpoint string `json:"href"`
	ID       ID     `json:"id"`
	Name     string `json:"name"`
	// A URL to a 30 second preview (MP3) of the track.
	PreviewURL string `json:"preview_url"`
	// The number of the track.  If an album has several
	// discs, the track number is the number on the specified
	// DiscNumber.
	TrackNumber int `json:"track_number"`
	URI         URI `json:"uri"`
}

// FullTrack provides extra track data in addition to what is provided by SimpleTrack.
type FullTrack struct {
	SimpleTrack
	// The album on which the track appears. The album object includes a link in href to full information about the album.
	Album SimpleAlbum `json:"album"`
	// Known external IDs for the track.
	ExternalIDs ExternalID `json:"external_ids"`
	// Popularity of the track.  The value will be between 0 and 100,
	// with 100 being the most popular.  The popularity is calculated from
	// both total plays and most recent plays.
	Popularity int `json:"popularity"`
}

// PlaylistTrack contains info about a track in a playlist.
type PlaylistTrack struct {
	// The date and time the track was added to the playlist.
	// You can use the TimestampLayout constant to convert
	// this field to a time.Time value.
	// Warning: very old playlists may not populate this value.
	AddedAt string `json:"added_at"`
	// The Spotify user who added the track to the playlist.
	// Warning: vary old playlists may not populate this value.
	AddedBy User `json:"added_by"`
	// Information about the track.
	Track FullTrack `json:"track"`
}

// SavedTrack provides info about a track saved to a user's account.
type SavedTrack struct {
	// The date and time the track was saved, represented as an ISO
	// 8601 UTC timestamp with a zero offset (YYYY-MM-DDTHH:MM:SSZ).
	// You can use the TimestampLayout constant to convert this to
	// a time.Time value.
	AddedAt   string `json:"added_at"`
	FullTrack `json:"track"`
}

// TimeDuration returns the track's duration as a time.Duration value.
func (t *SimpleTrack) TimeDuration() time.Duration {
	return time.Duration(t.Duration) * time.Millisecond
}

// GetTrack is a wrapper around DefaultClient.GetTrack.
func GetTrack(id ID) (*FullTrack, error) {
	return DefaultClient.GetTrack(id)
}

// GetTrack gets Spotify catalog information for
// a single track identified by its unique Spotify ID.
func (c *Client) GetTrack(id ID) (*FullTrack, error) {
	spotifyURL := baseAddress + "tracks/" + string(id)
	resp, err := c.http.Get(spotifyURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp.Body)
	}
	var t FullTrack
	err = json.NewDecoder(resp.Body).Decode(&t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTracks is a wrapper around DefaultClient.GetTracks.
func GetTracks(ids ...ID) ([]*FullTrack, error) {
	return DefaultClient.GetTracks(ids...)
}

// GetTracks gets Spotify catalog information for multiple tracks based on their
// Spotify IDs.  It supports up to 50 tracks in a single call.  Tracks are
// returned in the order requested.  If a track is not found, that position in the
// result will be nil.  Duplicate ids in the query will result in duplicate
// tracks in the result.
func (c *Client) GetTracks(ids ...ID) ([]*FullTrack, error) {
	if len(ids) > 50 {
		return nil, errors.New("spotify: FindTracks supports up to 50 tracks")
	}
	spotifyURL := baseAddress + "tracks?ids=" + strings.Join(toStringSlice(ids), ",")
	resp, err := c.http.Get(spotifyURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp.Body)
	}

	var t struct {
		Tracks []*FullTrack `jsosn:"tracks"`
	}
	err = json.NewDecoder(resp.Body).Decode(&t)
	if err != nil {
		return nil, errors.New("spotify:  couldn't decode tracks")
	}
	return t.Tracks, nil
}