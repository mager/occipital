package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

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

type SearchRequest struct {
	Query string `json:"query"`
}

type SearchResponse struct {
	Results []SearchTrack `json:"results"`
}

type SearchTrack struct {
	Artist     string `json:"artist"`
	Name       string `json:"name"`
	Popularity int    `json:"popularity"`
}

// ServeHTTP handles an HTTP request to the /spotify/search endpoint
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	var req SearchRequest
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate search query
	if req.Query == "" {
		http.Error(w, "missing search query", http.StatusBadRequest)
		return
	}

	h.log.Sugar().Infow("search", "query", req.Query)

	results, err := h.spotifyClient.Client.Search(ctx, req.Query, spot.SearchTypeTrack)
	if err != nil {
		http.Error(w, "search error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var resp SearchResponse

	if results.Tracks != nil {
		for _, item := range results.Tracks.Tracks {
			var t SearchTrack
			if len(item.Artists) > 0 {
				var artist strings.Builder
				for _, a := range item.Artists {
					artist.WriteString(a.Name)
				}
			}
			t.Name = item.Name
			t.Popularity = int(item.Popularity)
			resp.Results = append(resp.Results, t)
		}
	}

	json.NewEncoder(w).Encode(resp)
}
