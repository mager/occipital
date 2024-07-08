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
	seeds := spot.Seeds{
		Genres: []string{"pop"},
	}
	recs, err := h.spotifyClient.Client.GetRecommendations(ctx, seeds, nil, spot.Limit(10))
	if err != nil {
		http.Error(w, "featured playlist error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tracks := make([]occipital.Track, 0, 24)
	for _, track := range recs.Tracks {
		var t occipital.Track
		t.Name = track.Name
		t.Artist = util.GetFirstArtist(track.Artists)
		t.Source = "SPOTIFY"
		t.SourceID = string(track.ID)
		t.Image = *util.GetThumb(track.Album)
		tracks = append(tracks, t)
	}

	var resp RecommendedTracksResponse

	resp.Tracks = tracks

	json.NewEncoder(w).Encode(resp)
}
