package discover

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"
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

// sourceConfig defines a source and its scoring weight
type v2SourceConfig struct {
	collection string
	weight     float64
	maxRank    float64
}

// v2Sources defines all melodex sources with their scoring weights.
// Higher weight = better signal for fresh music discovery.
var v2Sources = []v2SourceConfig{
	{collection: "spotify_new_releases", weight: 1.0, maxRank: 100},
	{collection: "reddit_fresh", weight: 0.9, maxRank: 50},
	{collection: "hnhh", weight: 0.7, maxRank: 100},
	{collection: "pitchfork_bnm", weight: 0.6, maxRank: 20},
	{collection: "billboard", weight: 0.5, maxRank: 100},
}

func (h *DiscoverV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	now := time.Now()

	type scoredTrack struct {
		track       occipital.Track
		source      string
		rank        int
		weight      float64
		maxRank     float64
		sourceCount int
		score       float64
	}

	// Collect tracks from all sources, keyed by normalized artist+title
	trackMap := make(map[string]*scoredTrack)
	sourceMap := make(map[string]map[string]bool) // key -> set of sources
	artistCount := make(map[string]int)            // cap tracks per artist
	var latestDate string

	const maxTracksPerArtist = 2

	for _, src := range v2Sources {
		tracks, dateUsed, err := h.fetchTracksWithFallback(ctx, now, src.collection)
		if err != nil {
			h.log.Warnw("Failed to fetch source, skipping",
				"source", src.collection, "err", err)
			continue
		}

		h.log.Infow("Fetched tracks from source",
			"source", src.collection,
			"trackCount", len(tracks),
			"date", dateUsed,
		)

		if dateUsed > latestDate {
			latestDate = dateUsed
		}

		for _, fsTrack := range tracks {
			// Skip tracks without album art — they'd show as broken images
			if fsTrack.Thumb == "" {
				continue
			}
			oTrack := convertToOccipitalTrack(fsTrack, src.collection)
			oTrack.Source = src.collection
			key := normalizeTrackKey(fsTrack.Artist, fsTrack.Title)
			artistKey := normalizeArtist(fsTrack.Artist)

			if existing, ok := trackMap[key]; ok {
				// Track already seen from another source — keep the one with higher source weight
				if src.weight > existing.weight {
					trackMap[key] = &scoredTrack{
						track:   oTrack,
						source:  src.collection,
						rank:    fsTrack.Rank,
						weight:  src.weight,
						maxRank: src.maxRank,
					}
				}
			} else {
				// Skip if this artist already has enough tracks
				if artistCount[artistKey] >= maxTracksPerArtist {
					continue
				}
				artistCount[artistKey]++
				trackMap[key] = &scoredTrack{
					track:   oTrack,
					source:  src.collection,
					rank:    fsTrack.Rank,
					weight:  src.weight,
					maxRank: src.maxRank,
				}
			}

			// Track source appearances for cross-source bonus
			if sourceMap[key] == nil {
				sourceMap[key] = make(map[string]bool)
			}
			sourceMap[key][src.collection] = true
		}
	}

	// Score all tracks
	var results []scoredTrack
	for key, st := range trackMap {
		st.sourceCount = len(sourceMap[key])
		st.score = computeScore(st.rank, st.weight, st.maxRank, st.sourceCount)
		results = append(results, *st)
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Cap at 100 tracks
	const maxDiscoverTracks = 150
	if len(results) > maxDiscoverTracks {
		results = results[:maxDiscoverTracks]
	}

	// Convert to response
	finalTracks := make([]occipital.Track, 0, len(results))
	for _, r := range results {
		r.track.Rank = 0 // Clear source-specific rank; order IS the rank now
		finalTracks = append(finalTracks, r.track)
	}

	resp := &DiscoverResponse{
		Tracks:  finalTracks,
		Updated: latestDate,
	}

	h.log.Infow("Discover v2 response",
		"totalTracks", len(finalTracks),
		"updated", latestDate,
	)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Error encoding response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// computeScore implements the melodex v2 scoring algorithm.
//
//	score = (source_weight * normalized_rank²) + cross_source_bonus
//
// Uses exponential rank decay so top-ranked tracks score much higher.
// Cross-source bonus rewards tracks appearing on multiple charts.
func computeScore(rank int, weight, maxRank float64, sourceCount int) float64 {
	normalizedRank := 1.0 - (float64(rank) / maxRank)
	normalizedRank = math.Max(0.0, math.Min(1.0, normalizedRank))
	rankScore := weight * (normalizedRank * normalizedRank)

	crossSource := 0.0
	if sourceCount >= 3 {
		crossSource = 1.0
	} else if sourceCount >= 2 {
		crossSource = 0.5
	}

	return rankScore + crossSource
}

// normalizeArtist strips feat/ft variations and lowercases the artist name.
func normalizeArtist(artist string) string {
	a := strings.ToLower(strings.TrimSpace(artist))
	for _, sep := range []string{" feat.", " ft.", " featuring ", " feat ", " ft "} {
		if idx := strings.Index(a, sep); idx > 0 {
			a = a[:idx]
		}
	}
	return a
}

// normalizeTrackKey creates a lookup key for deduplication.
func normalizeTrackKey(artist, title string) string {
	return normalizeArtist(artist) + " - " + strings.ToLower(strings.TrimSpace(title))
}

// fetchTracksWithFallback tries today, then falls back up to 5 days.
func (h *DiscoverV2Handler) fetchTracksWithFallback(ctx context.Context, now time.Time, collection string) ([]fsClient.Track, string, error) {
	col := h.fs.Collection(collection)

	for i := 0; i <= maxDaysToLookBack; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		doc, err := col.Doc(date).Get(ctx)
		if err != nil {
			continue
		}
		var tracksDoc fsClient.TracksDoc
		if err := doc.DataTo(&tracksDoc); err != nil {
			h.log.Warnw("Error decoding tracks doc",
				"collection", collection, "date", date, "err", err)
			continue
		}
		if i > 0 {
			h.log.Infow("Using fallback date for source",
				"collection", collection, "date", date, "daysBack", i)
		}
		return tracksDoc.Tracks, date, nil
	}

	return nil, "", nil // No data found — not an error, just skip this source
}
