package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

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
	Artist     string  `json:"artist"`
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Popularity int     `json:"popularity"`
	Thumb      *string `json:"thumb"`
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
			resp.Results = append(resp.Results, mapTrack(item))
		}

		// Sorting by Popularity (descending order)
		sort.Slice(resp.Results, func(i, j int) bool {
			return resp.Results[i].Popularity > resp.Results[j].Popularity
		})
	}

	json.NewEncoder(w).Encode(resp)
}

func mapTrack(t spot.FullTrack) SearchTrack {
	var o SearchTrack

	o.Artist = spotify.GetFirstArtist(t.Artists)
	o.ID = string(t.ID)
	o.Name = t.Name
	o.Popularity = int(t.Popularity)

	o.Thumb = getThumb(t.Album)

	return o
}

func getThumb(a spot.SimpleAlbum) *string {
	var o string
	if len(a.Images) > 0 {
		// Find the smallest thumbnail
		smallestImage := a.Images[0]

		// Iterate through all images to find the smallest one
		for _, img := range a.Images {
			if img.Height < smallestImage.Height {
				smallestImage = img
			}
		}

		// Set the smallest image URL to the Thumb field
		o = smallestImage.URL
	}

	return &o
}
