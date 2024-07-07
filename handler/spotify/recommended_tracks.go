package spotify

import (
	"context"
	"encoding/json"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

// RecommendedTracksHandler is an http.Handler
type RecommendedTracksHandler struct {
	log           *zap.Logger
	spotifyClient *spotify.SpotifyClient
}

func (*RecommendedTracksHandler) Pattern() string {
	return "/spotify/recommended_tracks"
}

// NewRecommendedTracksHandler builds a new RecommendedTracksHandler.
func NewRecommendedTracksHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient) *RecommendedTracksHandler {
	return &RecommendedTracksHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type RecommendedTracksRequest struct {
}

type RecommendedTracksResponse struct {
	Tracks []occipital.Track `json:"tracks"`
}

// Get recommended tracks on Spotify
// @Summary Get recommended tracks on Spotify
// @Description Get the top featured tracks on Spotify
// @Tags Spotify
// @Accept json
// @Produce json
// @Success 200 {object} RecommendedTracksResponse
// @Router /spotify/recommended_tracks [get]
func (h *RecommendedTracksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := context.Background()
	_, p, err := h.spotifyClient.Client.FeaturedPlaylists(ctx, spot.Limit(10))
	if err != nil {
		http.Error(w, "featured playlist error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Iterate through the list of playlists to get the top 10 playlists
	playlistURIs := make([]spot.URI, 0, min(10, len(p.Playlists)))
	for _, playlist := range p.Playlists[:min(10, len(p.Playlists))] {
		playlistURIs = append(playlistURIs, playlist.URI)
	}

	// Fetch the 10 playlists and get the first 10 songs
	tracks := make([]occipital.Track, 0, 100)
	for _, playlistURI := range playlistURIs {
		pli, err := h.spotifyClient.Client.GetPlaylistItems(ctx, spotify.ExtractID(playlistURI), spot.Limit(10))
		if err != nil {
			http.Error(w, "error fetching playlist: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, track := range pli.Items {
			var t occipital.Track
			t.Name = track.Track.Track.Name
			t.Artist = util.GetFirstArtist(track.Track.Track.Artists)
			t.Source = "SPOTIFY"
			t.SourceID = string(track.Track.Track.ID)
			t.Image = *util.GetThumb(track.Track.Track.Album)
			tracks = append(tracks, t)
		}
	}

	var resp RecommendedTracksResponse

	resp.Tracks = tracks

	json.NewEncoder(w).Encode(resp)
}
