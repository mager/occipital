package spotify

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

// SearchHandler is an http.Handler that copies its request body
// back to the response.
type SearchHandler struct {
	log           *zap.Logger
	spotifyClient *spotify.SpotifyClient
}

func (*SearchHandler) Pattern() string {
	return "/spotify/search"
}

// NewSearchHandler builds a new SearchHandler.
func NewSearchHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient) *SearchHandler {
	return &SearchHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type Response struct {
	Results []Track `json:"results"`
}

type Track struct {
	Name string `json:"name"`
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	var resp Response
	w.Header().Set("Content-Type", "application/json")

	results, err := h.spotifyClient.Client.Search(ctx, "lunch", spot.SearchTypeTrack)
	if err != nil {
		log.Fatal(err)
	}

	if results.Tracks != nil {
		for _, item := range results.Tracks.Tracks {
			var t Track
			t.Name = item.Name
			resp.Results = append(resp.Results, t)
		}
	}

	json.NewEncoder(w).Encode(resp)
}
