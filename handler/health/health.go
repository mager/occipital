package health

import (
	"encoding/json"
	"net/http"

	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

// HealthHandler exposes only the server's liveness status.
type HealthHandler struct {
}

func (*HealthHandler) Pattern() string {
	return "/health"
}

// NewHealthHandler builds a new HealthHandler.
func NewHealthHandler(log *zap.SugaredLogger, spotifyClient *spotify.SpotifyClient) *HealthHandler {
	return &HealthHandler{}
}

type Response struct {
	Server bool `json:"server"`
}

// ServeHTTP handles the public liveness check.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Server: true})
}
