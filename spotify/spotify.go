package spotify

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mager/occipital/config"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type SpotifyClient struct {
	ID          string
	Secret      string
	Client      *spotify.Client
	TokenSource oauth2.TokenSource
}

func ProvideSpotify(cfg config.Config) *SpotifyClient {
	// Token refresh uses this context separately from API request contexts.
	// Give it a finite HTTP timeout as well.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})

	var c SpotifyClient

	c.ID = cfg.SpotifyID
	c.Secret = cfg.SpotifySecret

	// Create a client credentials config that will handle token refreshing
	config := &clientcredentials.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		TokenURL:     spotifyauth.TokenURL,
	}

	// Create a token source that will automatically refresh the token
	tokenSource := config.TokenSource(ctx)
	if _, err := tokenSource.Token(); err != nil {
		log.Fatalf("couldn't get token: %v", err)
	}

	// Create the client with the token source
	httpClient := oauth2.NewClient(ctx, tokenSource)
	httpClient.Timeout = 10 * time.Second
	client := spotify.New(httpClient)

	c.Client = client
	c.TokenSource = tokenSource

	return &c
}

var Options = ProvideSpotify
