package spotify

import (
	"context"
	"encoding/json"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

// GetFeaturedTracksHandler is an http.Handler
type GetFeaturedTracksHandler struct {
	log           *zap.SugaredLogger
	spotifyClient *spotify.SpotifyClient
}

func (*GetFeaturedTracksHandler) Pattern() string {
	return "/spotify/get_featured_tracks"
}

// NewGetFeaturedTracksHandler builds a new GetFeaturedTracksHandler.
func NewGetFeaturedTracksHandler(log *zap.SugaredLogger, spotifyClient *spotify.SpotifyClient) *GetFeaturedTracksHandler {
	return &GetFeaturedTracksHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type GetFeaturedTracksRequest struct {
}

type GetFeaturedTracksResponse struct {
	Tracks []FeaturedTrack `json:"tracks"`
}

type FeaturedTrack struct {
	Artist   string `json:"artist"`
	Name     string `json:"name"`
	SourceID string `json:"source_id"`
	Source   string `json:"source"`
	Image    string `json:"image"`
}

// Get featured tracks on Spotify
// @Summary Get featured tracks on Spotify
// @Description Get the top featured tracks on Spotify
// @Tags Spotify
// @Accept json
// @Produce json
// @Success 200 {object} GetFeaturedTracksResponse
// @Router /spotify/get_featured_tracks [get]
func (h *GetFeaturedTracksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	tracks := make([]FeaturedTrack, 0, 100)
	for _, playlistURI := range playlistURIs {
		pli, err := h.spotifyClient.Client.GetPlaylistItems(ctx, spotify.ExtractID(playlistURI), spot.Limit(10))
		if err != nil {
			http.Error(w, "error fetching playlist: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, track := range pli.Items {
			var t FeaturedTrack
			t.Name = track.Track.Track.Name
			t.Artist = util.GetFirstArtist(track.Track.Track.Artists)
			t.Source = "SPOTIFY"
			t.SourceID = string(track.Track.Track.ID)
			t.Image = *util.GetThumb(track.Track.Track.Album)
			tracks = append(tracks, t)
		}
	}

	var resp GetFeaturedTracksResponse

	resp.Tracks = tracks

	json.NewEncoder(w).Encode(resp)
}
