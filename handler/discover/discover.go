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
	"golang.org/x/exp/rand"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DiscoverHandler is an http.Handler
type DiscoverHandler struct {
	log           *zap.SugaredLogger
	fs            *firestore.Client
	spotifyClient *spotify.SpotifyClient
}

func (*DiscoverHandler) Pattern() string {
	return "/discover"
}

// NewDiscoverHandler builds a new DiscoverHandler.
func NewDiscoverHandler(log *zap.SugaredLogger, fs *firestore.Client, spotifyClient *spotify.SpotifyClient) *DiscoverHandler {
	return &DiscoverHandler{
		log:           log,
		fs:            fs,
		spotifyClient: spotifyClient,
	}
}

type DiscoverRequest struct {
}

type DiscoverResponse struct {
	Tracks    []occipital.Track `json:"tracks"`
	Billboard []occipital.Track `json:"billboard"`
	HypeM     []occipital.Track `json:"hypem"`
}

// Function to convert fsClient.Track to occipital.Track
func convertToOccipitalTrack(fsTrack fsClient.Track, thumbType string) occipital.Track {
	var image string
	if thumbType == "hypem" {
		image = getHypemThumb(fsTrack.Thumb)
	} else {
		image = getSpotifyThumb(fsTrack.Thumb)
	}
	return occipital.Track{
		Artist:   fsTrack.Artist,
		Name:     fsTrack.Title,
		Source:   "SPOTIFY",
		SourceID: fsTrack.SpotifyID,
		Image:    image,
	}
}

func getSpotifyThumb(th string) string {
	return fmt.Sprintf("https://i.scdn.co/image/%s", th)
}

func getHypemThumb(th string) string {
	return fmt.Sprintf("https://static.hypem.com/items_images/%s", th)
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

	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Error decoding request body", zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Fetch Billboard
	billboardDoc, err := h.fs.Collection("billboard").Doc(today).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			h.log.Warn("Document not found for today, searching for yesterday")
			billboardDoc, err = h.fs.Collection("billboard").Doc(yesterday).Get(ctx)
		}
		if err != nil {
			h.log.Error("Error fetching document snapshot", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var billboardTracksDoc fsClient.TracksDoc
	if err := billboardDoc.DataTo(&billboardTracksDoc); err != nil {
		h.log.Error("Error converting document snapshot to tracks", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	billboardTracks := []occipital.Track{}
	for i, fsTrack := range billboardTracksDoc.Tracks {
		if i >= 48 {
			break
		}
		billboardTracks = append(billboardTracks, convertToOccipitalTrack(fsTrack, "spotify"))
	}
	// Fetch Hype Machine
	hypemDoc, err := h.fs.Collection("hypem").Doc(today).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			h.log.Warn("Document not found for today, searching for yesterday")
			hypemDoc, err = h.fs.Collection("hypem").Doc(yesterday).Get(ctx)
		}
		if err != nil {
			h.log.Error("Error fetching document snapshot", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var hypemTracksDoc fsClient.TracksDoc
	if err := hypemDoc.DataTo(&hypemTracksDoc); err != nil {
		h.log.Error("Error converting document snapshot to tracks", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hypemTracks := []occipital.Track{}
	for i, fsTrack := range hypemTracksDoc.Tracks {
		if i >= 48 {
			break
		}
		hypemTracks = append(hypemTracks, convertToOccipitalTrack(fsTrack, "hypem"))
	}

	// If fewer than 48 hypem tracks, fill with additional billboard tracks
	neededTracks := 48 - len(hypemTracks)
	if neededTracks > 0 {
		for i := 48; i < len(billboardTracksDoc.Tracks) && len(hypemTracks) < 48; i++ {
			hypemTracks = append(hypemTracks, convertToOccipitalTrack(billboardTracksDoc.Tracks[i], "spotify"))
		}
	}

	// Combine and shuffle the tracks
	tracks := append(billboardTracks, hypemTracks...)
	rand.Seed(uint64(time.Now().UnixNano())) // Seed the random number generator
	rand.Shuffle(len(tracks), func(i, j int) {
		tracks[i], tracks[j] = tracks[j], tracks[i]
	})

	// Create the response
	resp := &DiscoverResponse{
		Tracks: tracks,
	}

	json.NewEncoder(w).Encode(resp)
}
