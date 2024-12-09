package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	fsClient "github.com/mager/occipital/firestore"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

// DiscoverHandler is an http.Handler
type DiscoverHandler struct {
	log           *zap.Logger
	fs            *firestore.Client
	spotifyClient *spotify.SpotifyClient
}

func (*DiscoverHandler) Pattern() string {
	return "/discover"
}

// NewDiscoverHandler builds a new DiscoverHandler.
func NewDiscoverHandler(log *zap.Logger, fs *firestore.Client, spotifyClient *spotify.SpotifyClient) *DiscoverHandler {
	return &DiscoverHandler{
		log:           log,
		fs:            fs,
		spotifyClient: spotifyClient,
	}
}

type DiscoverRequest struct {
}

type DiscoverResponse struct {
	Tracks []occipital.Track `json:"tracks"`
}

// Function to convert fsClient.Track to occipital.Track
func convertToOccipitalTrack(fsTrack fsClient.Track) occipital.Track {
	return occipital.Track{
		Artist:   fsTrack.Artist,
		Name:     fsTrack.Title,
		Source:   "SPOTIFY",
		SourceID: fsTrack.SpotifyID,
		Image:    fmt.Sprintf("https://i.scdn.co/image/%s", fsTrack.Thumb),
	}
}

// Discover
// @Summary Home page
// @Description Get the best content
// @Accept json
// @Produce json
// @Success 200 {object} DiscoverResponse
// @Router /discover [post]
func (h *DiscoverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")
	h.log.Debug("Received request", zap.String("method", r.Method), zap.String("url", r.URL.String())) // Debug log for incoming request

	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Error decoding request body", zap.Error(err)) // Log error if decoding fails
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	datestr := time.Now().Format("2006-01-02")
	docsnap, err := h.fs.Collection("billboard").Doc(datestr).Get(ctx)
	if err != nil {
		h.log.Error("Error fetching document snapshot", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var tracksDoc fsClient.TracksDoc
	if err := docsnap.DataTo(&tracksDoc); err != nil {
		h.log.Error("Error converting document snapshot to tracks", zap.Error(err)) // Log error if conversion fails
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Map fsTracks to occipital.Tracks
	var tracks []occipital.Track
	for _, fsTrack := range tracksDoc.Tracks {
		tracks = append(tracks, convertToOccipitalTrack(fsTrack))
	}

	json.NewEncoder(w).Encode(tracks)
}
