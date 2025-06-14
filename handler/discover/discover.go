package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	fsClient "github.com/mager/occipital/firestore"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
	"golang.org/x/exp/rand"
)

const maxDaysToLookBack = 5

// DiscoverHandler is an http.Handler
type DiscoverHandler struct {
	log           *zap.SugaredLogger
	fs            *firestore.Client
	spotifyClient *spotify.SpotifyClient
}

func (*DiscoverHandler) Pattern() string {
	return "/discover/v1"
}

func (*DiscoverHandler) PatternV2() string {
	return "/discover/v2"
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
	case "hnhh":
		image = getHnhhThumb(fsTrack.Thumb)
	default:
		image = getSpotifyThumb(fsTrack.Thumb)
	}
	return occipital.Track{
		Artist:   fsTrack.Artist,
		Name:     fsTrack.Title,
		SourceID: fsTrack.SpotifyID,
		Image:    image,
		ID:       fsTrack.MBID,
		ISRC:     fsTrack.ISRC,
		Rank:     fsTrack.Rank,
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

	maxTotalTracks := 150
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

	// Log final request details
	h.log.Infow(
		"Discover request finished",
		"mode", req.Mode,
		"tracksPerSource", tracksPerSource,
		"totalTracks", len(allTracks),
		"updatedDate", latestDate,
	)

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

	// Try today first
	doc, err := col.Doc(today).Get(ctx)
	if err == nil {
		return h.extractTracks(doc, today)
	}

	// If today fails, try previous days up to maxDaysToLookBack
	for i := 1; i <= maxDaysToLookBack; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		doc, err = col.Doc(date).Get(ctx)
		if err == nil {
			h.log.Infow("Found tracks from previous day",
				"collection", collectionName,
				"date", date,
				"daysBack", i)
			return h.extractTracks(doc, date)
		}
	}

	// If we get here, we couldn't find any documents
	h.log.Error("Failed to fetch document from any recent date",
		"collection", collectionName,
		"maxDaysBack", maxDaysToLookBack)
	return nil, "", fmt.Errorf("error fetching document snapshot from collection '%s': no data found in last %d days", collectionName, maxDaysToLookBack)
}

func (h *DiscoverHandler) extractTracks(doc *firestore.DocumentSnapshot, date string) ([]fsClient.Track, string, error) {
	var tracksDoc fsClient.TracksDoc
	if err := doc.DataTo(&tracksDoc); err != nil {
		h.log.Error("Failed to convert document to tracks", zap.String("date", date), zap.Error(err))
		return nil, "", fmt.Errorf("error converting document snapshot to tracks: %w", err)
	}
	return tracksDoc.Tracks, date, nil
}

// ServeHTTPV2 handles the v2 discover endpoint
func (h *DiscoverHandler) ServeHTTPV2(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("Error decoding request body", zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Define all sources
	sources := []sourceConfig{
		{"billboard", "spotify"},
		{"hypem", "hypem"},
		{"hnhh", "hnhh"},
	}

	var allTracks []occipital.Track
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	var latestDate string
	for _, source := range sources {
		tracks, dateUsed, err := h.fetchTracksFromSource(ctx, today, yesterday, source.name, source.thumbType)
		if err != nil {
			h.log.Error("Error fetching tracks from source", zap.String("source", source.name), zap.Error(err))
			http.Error(w, "Failed to fetch tracks", http.StatusInternalServerError)
			return
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

	sort.Slice(allTracks, func(i, j int) bool {
		return allTracks[i].Rank < allTracks[j].Rank
	})

	resp := &DiscoverResponse{
		Tracks:  allTracks,
		Updated: latestDate,
	}

	// Log final request details
	h.log.Infow(
		"Discover v2 request finished",
		"totalTracks", len(allTracks),
		"updatedDate", latestDate,
	)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Error encoding response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// getRankFromSource returns the rank based on the source
func getRankFromSource(source string) int {
	switch source {
	case "billboard":
		return 1
	case "hypem":
		return 2
	case "hnhh":
		return 3
	default:
		return 999
	}
}
