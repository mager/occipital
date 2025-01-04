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
	Popular int `json:"popular"`
}

type DiscoverResponse struct {
	Tracks []occipital.Track `json:"tracks"`
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

	sources := []struct {
		name      string
		thumbType string
	}{
		{"billboard", "spotify"},
		{"hypem", "hypem"},
		{"hnhh", "hnhh"},
	}

	var allTracks []occipital.Track
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	maxTotalTracks := 100
	billboardPercentage := req.Popular
	if billboardPercentage < 0 {
		billboardPercentage = 0
	} else if billboardPercentage > 100 {
		billboardPercentage = 100
	}

	billboardMaxTracks := maxTotalTracks * billboardPercentage / 100
	otherMaxTracks := (maxTotalTracks - billboardMaxTracks) / (len(sources) - 1)

	for _, source := range sources {
		var maxTracks int
		if source.name == "billboard" {
			maxTracks = billboardMaxTracks
		} else {
			maxTracks = otherMaxTracks
		}

		tracks, err := h.fetchTracksFromSource(ctx, today, yesterday, source.name, source.thumbType)
		if err != nil {
			h.log.Error("Error fetching tracks", zap.Error(err), zap.String("source", source.name))
			http.Error(w, "Failed to fetch tracks", http.StatusInternalServerError)
			return
		}
		if len(tracks) > maxTracks {
			tracks = tracks[:maxTracks]
		}
		allTracks = append(allTracks, tracks...)
	}

	rand.Shuffle(len(allTracks), func(i, j int) {
		allTracks[i], allTracks[j] = allTracks[j], allTracks[i]
	})

	// Limit to a maximum of 100 tracks
	if len(allTracks) > maxTotalTracks {
		allTracks = allTracks[:maxTotalTracks]
	}

	resp := &DiscoverResponse{
		Tracks: allTracks,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Error encoding response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (h *DiscoverHandler) fetchTracksFromSource(ctx context.Context, today, yesterday, collectionName, thumbType string) ([]occipital.Track, error) {
	tracksDoc, err := h.fetchTracksFromCollection(ctx, today, yesterday, collectionName)
	if err != nil {
		return nil, err
	}
	tracks := make([]occipital.Track, 0, len(tracksDoc))
	for _, fsTrack := range tracksDoc {
		tracks = append(tracks, convertToOccipitalTrack(fsTrack, thumbType))
	}
	return tracks, nil
}

func (h *DiscoverHandler) fetchTracksFromCollection(ctx context.Context, today, yesterday, collectionName string) ([]fsClient.Track, error) {
	col := h.fs.Collection(collectionName)
	doc, err := col.Doc(today).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			h.log.Warnf("Document not found for today in collection '%s', searching for yesterday", collectionName)
			doc, err = col.Doc(yesterday).Get(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("error fetching document snapshot from collection '%s': %w", collectionName, err)
		}
	}

	var tracksDoc fsClient.TracksDoc
	if err := doc.DataTo(&tracksDoc); err != nil {
		return nil, fmt.Errorf("error converting document snapshot to tracks for collection '%s': %w", collectionName, err)
	}

	return tracksDoc.Tracks, nil
}
