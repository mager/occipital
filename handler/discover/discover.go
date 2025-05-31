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
	Mode string `json:"mode"`
}

type DiscoverResponse struct {
	Tracks  []occipital.Track `json:"tracks"`
	Updated string            `json:"updated"`
}

// Define a named struct for source configuration
type sourceConfig struct {
	name      string
	thumbType string
}

// Function to convert fsClient.Track to occipital.Track
func convertToOccipitalTrack(fsTrack fsClient.Track, thumbType string) occipital.Track {
	var image string
	switch thumbType {
	case "hypem":
		image = getHypemThumb(fsTrack.Thumb)
	case "hnhh":
		image = getHnhhThumb(fsTrack.Thumb)
	default:
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

func getHnhhThumb(th string) string {
	return getSpotifyThumb(th)
}

// Discover
// @Summary Home page
// @Description Get the best content
// @Accept json
// @Produce json
// @Param request body DiscoverRequest true "Request parameters"
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

	h.log.Infow("Discover request received", "mode", req.Mode)

	maxTotalTracks := 100
	var sources []sourceConfig
	var tracksPerSource map[string]int

	switch req.Mode {
	case "hot":
		sources = []sourceConfig{{"billboard", "spotify"}}
		tracksPerSource = map[string]int{"billboard": maxTotalTracks}
	case "new":
		sources = []sourceConfig{{"hypem", "hypem"}, {"hnhh", "hnhh"}}
		tracksPerSource = map[string]int{"hypem": maxTotalTracks / 2, "hnhh": maxTotalTracks / 2}
	default:
		sources = []sourceConfig{{"billboard", "spotify"}, {"hypem", "hypem"}, {"hnhh", "hnhh"}}
		tracksPerSource = map[string]int{"billboard": maxTotalTracks / 3, "hypem": maxTotalTracks / 3, "hnhh": maxTotalTracks / 3}
	}

	h.log.Infow(
		"Track allocation",
		"tracksPerSource", tracksPerSource,
	)

	var allTracks []occipital.Track
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	var latestDate string
	for _, source := range sources {
		maxTracks := tracksPerSource[source.name]

		tracks, dateUsed, err := h.fetchTracksFromSource(ctx, today, yesterday, source.name, source.thumbType)
		if err != nil {
			h.log.Error("Error fetching tracks from source", zap.String("source", source.name), zap.Error(err))
			http.Error(w, "Failed to fetch tracks", http.StatusInternalServerError)
			return
		}

		if len(tracks) > maxTracks {
			tracks = tracks[:maxTracks]
		}
		allTracks = append(allTracks, tracks...)

		if dateUsed > latestDate {
			latestDate = dateUsed
		}
	}

	// Deduplicate tracks by artist
	uniqueTracks := make([]occipital.Track, 0, len(allTracks))
	artistSeen := make(map[string]bool)
	for _, track := range allTracks {
		if !artistSeen[track.Artist] {
			uniqueTracks = append(uniqueTracks, track)
			artistSeen[track.Artist] = true
		}
	}
	allTracks = uniqueTracks

	rand.Shuffle(len(allTracks), func(i, j int) {
		allTracks[i], allTracks[j] = allTracks[j], allTracks[i]
	})

	if len(allTracks) > maxTotalTracks {
		allTracks = allTracks[:maxTotalTracks]
	}

	resp := &DiscoverResponse{
		Tracks:  allTracks,
		Updated: latestDate,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Error encoding response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (h *DiscoverHandler) fetchTracksFromSource(ctx context.Context, today, yesterday, collectionName, thumbType string) ([]occipital.Track, string, error) {
	tracksDoc, dateUsed, err := h.fetchTracksFromCollection(ctx, today, yesterday, collectionName)
	if err != nil {
		return nil, "", err
	}
	tracks := make([]occipital.Track, 0, len(tracksDoc))
	for _, fsTrack := range tracksDoc {
		tracks = append(tracks, convertToOccipitalTrack(fsTrack, thumbType))
	}
	return tracks, dateUsed, nil
}

func (h *DiscoverHandler) fetchTracksFromCollection(ctx context.Context, today, yesterday, collectionName string) ([]fsClient.Track, string, error) {
	col := h.fs.Collection(collectionName)

	doc, err := col.Doc(today).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			h.log.Warn("Today's document not found, attempting yesterday", zap.String("collection", collectionName), zap.String("date", yesterday))
			doc, err = col.Doc(yesterday).Get(ctx)
			if err != nil {
				h.log.Error("Failed to fetch yesterday's document", zap.String("collection", collectionName), zap.Error(err))
				return nil, "", fmt.Errorf("error fetching document snapshot from collection '%s': %w", collectionName, err)
			}
			return h.extractTracks(doc, yesterday)
		}
		h.log.Error("Error fetching today's document", zap.String("collection", collectionName), zap.Error(err))
		return nil, "", fmt.Errorf("error fetching document snapshot from collection '%s': %w", collectionName, err)
	}

	return h.extractTracks(doc, today)
}

func (h *DiscoverHandler) extractTracks(doc *firestore.DocumentSnapshot, date string) ([]fsClient.Track, string, error) {
	var tracksDoc fsClient.TracksDoc
	if err := doc.DataTo(&tracksDoc); err != nil {
		h.log.Error("Failed to convert document to tracks", zap.String("date", date), zap.Error(err))
		return nil, "", fmt.Errorf("error converting document snapshot to tracks: %w", err)
	}
	return tracksDoc.Tracks, date, nil
}
