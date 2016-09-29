package deathguild

import (
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

func GetSpotifyClient(clientID, clientSecret, refreshToken string) *spotify.Client {
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
