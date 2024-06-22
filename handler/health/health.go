package health

import (
	"encoding/json"
	"net/http"

	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

// HealthHandler is an http.Handler that copies its request body
// back to the response.
type HealthHandler struct {
	log           *zap.Logger
	spotifyClient *spotify.SpotifyClient
}

func (*HealthHandler) Pattern() string {
	return "/health"
}

// NewHealthHandler builds a new HealthHandler.
func NewHealthHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient) *HealthHandler {
	return &HealthHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type Response struct {
	Server  bool `json:"server"`
	Spotify bool `json:"spotify"`
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp Response

	h.log.Info("health check")

	resp.Server = true

	// Make sure Spotify client is set up properly
	if h.spotifyClient.ID != "" && h.spotifyClient.Secret != "" {
		resp.Spotify = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
