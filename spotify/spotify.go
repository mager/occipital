package spotify

import (
	"context"
	"log"

	"github.com/mager/occipital/config"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

type SpotifyClient struct {
	ID     string
	Secret string
	Client *spotify.Client
}

func ProvideSpotify(cfg config.Config) *SpotifyClient {
	ctx := context.Background()

	var c SpotifyClient

	c.ID = cfg.SpotifyID
	c.Secret = cfg.SpotifySecret

	config := &clientcredentials.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		TokenURL:     spotifyauth.TokenURL,
	}
	token, err := config.Token(ctx)
	if err != nil {
		log.Fatalf("couldn't get token: %v", err)
	}

	httpClient := spotifyauth.New().Client(ctx, token)
	client := spotify.New(httpClient)
	c.Client = client

	return &c
}

var Options = ProvideSpotify
