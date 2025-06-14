package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	fsClient "github.com/mager/occipital/firestore"
	"github.com/mager/occipital/occipital"
	"go.uber.org/zap"
)

type DiscoverV2Handler struct {
	log *zap.SugaredLogger
	fs  *firestore.Client
}

func NewDiscoverV2Handler(log *zap.SugaredLogger, fs *firestore.Client) *DiscoverV2Handler {
	return &DiscoverV2Handler{log: log, fs: fs}
}

func (h *DiscoverV2Handler) Pattern() string {
	return "/discover/v2"
}

func (h *DiscoverV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	sources := []string{
		"billboard",
		"hypem",
		"hnhh",
	}

	today := time.Now().Format("2006-01-02")
	var allTracks []occipital.Track
	var latestDate string

	for _, sourceName := range sources {
		col := h.fs.Collection(sourceName)
		doc, err := col.Doc(today).Get(ctx)
		if err != nil {
			h.log.Errorw("Error fetching today's tracks", "source", sourceName, "err", err)
			continue
		}
		var tracksDoc fsClient.TracksDoc
		if err := doc.DataTo(&tracksDoc); err != nil {
			h.log.Errorw("Error decoding tracks doc", "source", sourceName, "err", err)
			continue
		}
		for _, fsTrack := range tracksDoc.Tracks {
			track := convertToOccipitalTrack(fsTrack, sourceName)
			allTracks = append(allTracks, track)
		}
		if today > latestDate {
			latestDate = today
		}
	}

	// Sort by rank (ascending)
	sort.Slice(allTracks, func(i, j int) bool {
		return allTracks[i].Rank < allTracks[j].Rank
	})

	// Deduplicate by artist
	uniqueTracks := make([]occipital.Track, 0, len(allTracks))
	artistSeen := make(map[string]bool)
	for _, track := range allTracks {
		if !artistSeen[track.Artist] {
			uniqueTracks = append(uniqueTracks, track)
			artistSeen[track.Artist] = true
		}
	}

	resp := &DiscoverResponse{
		Tracks:  uniqueTracks,
		Updated: latestDate,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Error encoding response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
