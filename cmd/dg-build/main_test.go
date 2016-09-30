package main

import (
	"io/ioutil"
	"testing"

	"github.com/brandur/deathguild"
	assert "github.com/stretchr/testify/require"
)

func TestCompileJavascripts(t *testing.T) {
	dir, err := ioutil.TempDir("", "javascripts")

	file0 := dir + "/.hidden"
	file1 := dir + "/file1.js"
	file2 := dir + "/file2.js"
	file3 := dir + "/file3.js"
	out := dir + "/app.js"

	// This file is hidden and doesn't show up in output.
	err = ioutil.WriteFile(file0, []byte(`hidden`), 0755)
	assert.NoError(t, err)

	err = ioutil.WriteFile(file1, []byte(`function() { return "file1" }`), 0755)
	assert.NoError(t, err)

	err = ioutil.WriteFile(file2, []byte(`function() { return "file2" }`), 0755)
	assert.NoError(t, err)

	err = ioutil.WriteFile(file3, []byte(`function() { return "file3" }`), 0755)
	assert.NoError(t, err)

	err = compileJavascripts(dir, out)
	assert.NoError(t, err)

	actual, err := ioutil.ReadFile(out)
	assert.NoError(t, err)

	expected := `/* file1.js */

(function() {

function() { return "file1" }

}).call(this);

/* file2.js */

(function() {

function() { return "file2" }

}).call(this);

/* file3.js */

(function() {

function() { return "file3" }

}).call(this);

`
	assert.Equal(t, expected, string(actual))
}

func TestCompileStylesheets(t *testing.T) {
	dir, err := ioutil.TempDir("", "stylesheets")

	file0 := dir + "/.hidden"
	file1 := dir + "/file1.sass"
	file2 := dir + "/file2.sass"
	file3 := dir + "/file3.css"
	out := dir + "/app.css"

	// This file is hidden and doesn't show up in output.
	err = ioutil.WriteFile(file0, []byte("hidden"), 0755)
	assert.NoError(t, err)

	// The syntax of the first and second files is GCSS and the third is in
	// CSS.
	err = ioutil.WriteFile(file1, []byte("p\n  margin: 10px"), 0755)
	assert.NoError(t, err)

	err = ioutil.WriteFile(file2, []byte("p\n  padding: 10px"), 0755)
	assert.NoError(t, err)

	err = ioutil.WriteFile(file3, []byte("p {\n  border: 10px;\n}"), 0755)
	assert.NoError(t, err)

	err = compileStylesheets(dir, out)
	assert.NoError(t, err)

	actual, err := ioutil.ReadFile(out)
	assert.NoError(t, err)

	// Note that the first two files have no spacing in the output because they
	// go through the GCSS compiler.
	expected := `/* file1.sass */

p{margin:10px;}

/* file2.sass */

p{padding:10px;}

/* file3.css */

p {
  border: 10px;
}

`
	assert.Equal(t, expected, string(actual))
}

func TestIsHidden(t *testing.T) {
	assert.Equal(t, true, isHidden(".gitkeep"))
	assert.Equal(t, false, isHidden("article"))
}

func TestPlaylistInfo(t *testing.T) {
	playlist := &deathguild.Playlist{
		Songs: []*deathguild.Song{
			{Artist: "Depeche Mode", Title: "Two Minute Warning", SpotifyID: "spotify-id"},
			{Artist: "Imperative Reaction", Title: "You Remain"},
		},
		SpotifyID: "spotify-id",
	}

	assert.Equal(t, "2 song(s). 1 song(s) (50.0%) found in Spotify.",
		playlistInfo(playlist))
}

func TestSpotifyPlaylistLink(t *testing.T) {
	conf.SpotifyUser = "fyrerise"
	playlist := &deathguild.Playlist{SpotifyID: "spotify-id"}

	assert.Equal(t, "https://open.spotify.com/user/fyrerise/playlist/spotify-id",
		spotifyPlaylistLink(playlist))
}
