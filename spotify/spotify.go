package spotify

import (
	"github.com/mager/occipital/config"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

const redirectURI = "http://localhost:8080/callback"

var (
	auth  = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI), spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate))
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

type SpotifyClient struct {
	ID     string
	Secret string
}

func ProvideSpotify(cfg config.Config) *SpotifyClient {
	var c SpotifyClient

	c.ID = cfg.SpotifyID
	c.Secret = cfg.SpotifySecret

	return &c
}

var Options = ProvideSpotify
