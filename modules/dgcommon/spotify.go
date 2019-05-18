package dgcommon

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

// GetSpotifyClient returns a client that should be immediately useful for
// use. It takes advantage of the fact that we can just refresh right away to
// get a valid access token.
func GetSpotifyClient(clientID, clientSecret, refreshToken string) *spotify.Client {
	// Disables HTTP/2 support. It seems that Spotify might think that it
	// supports it, but we're unable to properly open a stream (as of January
	// 2017). Kill it off for now with the possibility of re-enabling it later.
	http.DefaultTransport.(*http.Transport).TLSNextProto =
		map[string]func(authority string, c *tls.Conn) http.RoundTripper{}

	// So as not to introduce a web flow into this program, we cheat a bit here
	// by just using a refresh token and not an access token (because access
	// tokens expiry very quickly and are therefore not suitable for inclusion
	// in configuration). This will force a refresh on the first call, but meh.
	token := new(oauth2.Token)
	token.Expiry = time.Now().Add(time.Second * -1)
	token.RefreshToken = refreshToken

	// See comment above. We've already procured the first access/refresh token
	// pair outside of this program, so no redirect URL is necessary.
	authenticator := spotify.NewAuthenticator("no-redirect-url")
	authenticator.SetAuthInfo(clientID, clientSecret)
	client := authenticator.NewClient(token)
	return &client
}
