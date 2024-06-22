package spotify

import (
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
	"honnef.co/go/tools/config"
)

const redirectURI = "http://localhost:8080/callback"

var (
	auth  = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI), spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate))
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

type SpotifyClient struct {
	Client *spotify.Client
	ID     string
	Secret string
}

func ProvideSpotify(cfg config.Config, log zap.Logger) *SpotifyClient {
	var c SpotifyClient

	log.Info("setting up spotify client")

	return &c
}

var Options = ProvideSpotify
