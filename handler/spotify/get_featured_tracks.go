package spotify

import (
	"context"
	"encoding/json"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

// GetFeaturedTracksHandler is an http.Handler
type GetFeaturedTracksHandler struct {
	log           *zap.Logger
	spotifyClient *spotify.SpotifyClient
}

func (*GetFeaturedTracksHandler) Pattern() string {
	return "/spotify/get_featured_tracks"
}

// NewGetFeaturedTracksHandler builds a new GetFeaturedTracksHandler.
func NewGetFeaturedTracksHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient) *GetFeaturedTracksHandler {
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
	Artist string `json:"artist"`
	Name   string `json:"name"`
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
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
	h.log.Sugar().Infow("test playlists", "playlist uris", playlistURIs, "len", len(playlistURIs))

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
			t.Artist = spotify.GetFirstArtist(track.Track.Track.Artists)
			tracks = append(tracks, t)

			h.log.Sugar().Infow("adding song", "title", t.Name, "artist", t.Artist)
		}
	}
	h.log.Sugar().Infow("test tracks", "len", len(tracks))

	var resp GetFeaturedTracksResponse

	resp.Tracks = tracks

	json.NewEncoder(w).Encode(resp)
}
