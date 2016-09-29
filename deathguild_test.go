package deathguild

import (
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
)

func TestPlaylistFormattedDay(t *testing.T) {
	const longForm = "Jan 2, 2006 at 3:04pm (MST)"
	day, err := time.Parse(longForm, "Feb 3, 2013 at 7:54pm (PST)")
	assert.NoError(t, err)

	p := Playlist{Day: day}
	assert.Equal(t, "2013-02-03", p.FormattedDay())
}
