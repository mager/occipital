package track

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

// GetTrackHandler is an http.Handler
type GetTrackHandler struct {
	log           *zap.Logger
	spotifyClient *spotify.SpotifyClient
}

func (*GetTrackHandler) Pattern() string {
	return fmt.Sprintf("/track")
}

// NewGetTrackHandler builds a new GetTrackHandler.
func NewGetTrackHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient) *GetTrackHandler {
	return &GetTrackHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type GetTrackRequest struct {
	SourceID string `json:"source_id"`
	Source   string `json:"source"`
}

type GetTrackResponse struct {
	Track Track `json:"track"`
}

type Track struct {
	Artist   string `json:"artist"`
	Name     string `json:"name"`
	SourceID string `json:"source_id"`
	Source   string `json:"source"`
	Image    string `json:"image"`
}

// Get track
// @Summary Get track
// @Description Get track
// @Accept json
// @Produce json
// @Success 200 {object} GetTrackResponse
// @Router /track [get]
func (h *GetTrackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
	source := q.Get("source")

	t, err := h.spotifyClient.Client.GetTrack(ctx, spot.ID(sourceId))
	if err != nil {
		http.Error(w, "featured playlist error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.log.Sugar().Infow("req", zap.String("sourceId", sourceId), zap.String("source", source))
	h.log.Sugar().Infow("resp", zap.Any("t", t))

	var resp GetTrackResponse
	var track Track

	track.Name = t.Name
	track.Artist = util.GetFirstArtist(t.Artists)
	track.SourceID = sourceId
	track.Source = source
	track.Image = *util.GetThumb(t.Album)

	resp.Track = track

	json.NewEncoder(w).Encode(resp)
}
