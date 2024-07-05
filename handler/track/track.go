package track

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mager/occipital/spotify"
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
	// ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
	source := q.Get("source")

	h.log.Sugar().Infow("req", zap.String("sourceId", sourceId), zap.String("source", source))
	// _, p, err := h.spotifyClient.Client.GetTrack(ctx, spot.ID())
	// if err != nil {
	// 	http.Error(w, "featured playlist error: "+err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	var resp GetTrackResponse

	var t Track
	resp.Track = t

	json.NewEncoder(w).Encode(resp)
}
