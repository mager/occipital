package track

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

const (
	trackCacheCollection = "track_cache_v2"
	trackCacheTTL        = 7 * 24 * time.Hour
)

type cachedTrackDoc struct {
	TrackJSON string    `firestore:"track_json"`
	CachedAt  time.Time `firestore:"cached_at"`
}

// GetTrackV2Handler is a fast, parallel, cached track handler.
type GetTrackV2Handler struct {
	log               *zap.SugaredLogger
	spotifyClient     *spotify.SpotifyClient
	musicbrainzClient *musicbrainz.MusicbrainzClient
	db                *firestore.Client
}

func (*GetTrackV2Handler) Pattern() string {
	return "/v2/track"
}

func NewGetTrackV2Handler(
	log *zap.SugaredLogger,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
	db *firestore.Client,
) *GetTrackV2Handler {
	return &GetTrackV2Handler{
		log:               log,
		spotifyClient:     spotifyClient,
		musicbrainzClient: musicbrainzClient,
		db:                db,
	}
}

// ServeHTTP handles GET /v2/track?spotifyId=xxx
func (h *GetTrackV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()
	q := r.URL.Query()
	spotifyId := q.Get("spotifyId")

	if spotifyId == "" {
		http.Error(w, `{"error":"spotifyId required"}`, http.StatusBadRequest)
		return
	}

	// --- Cache check ---
	if track, ok := h.getFromCache(ctx, spotifyId); ok {
		h.log.Infow("Cache hit", "spotify_id", spotifyId)
		json.NewEncoder(w).Encode(GetTrackResponse{Track: *track})
		return
	}

	h.log.Infow("Cache miss — fetching", "spotify_id", spotifyId)
	track := h.fetchParallel(ctx, spotifyId)

	// Fire-and-forget cache write
	go h.saveToCache(context.Background(), spotifyId, &track)

	json.NewEncoder(w).Encode(GetTrackResponse{Track: track})
}

// fetchParallel fans out all external calls optimally:
//
//	t=0 → Spotify: GetTrack, GetAudioFeatures, GetAudioAnalysis (all concurrent)
//	t=ISRC → MusicBrainz: SearchByISRC → GetRecording → GetWork (starts as soon as GetTrack returns ISRC)
func (h *GetTrackV2Handler) fetchParallel(ctx context.Context, spotifyId string) occipital.Track {
	l := h.log
	sid := spot.ID(spotifyId)

	// isrcCh carries the ISRC from GetTrack to the MB goroutine.
	// Buffered so the Spotify goroutine never blocks.
	isrcCh := make(chan string, 1)

	var (
		mu          sync.Mutex
		fullTrack   *spot.FullTrack
		audioFeats  []*spot.AudioFeatures
		audioAnal   *spot.AudioAnalysis
		mbRecording *mb.GetRecordingResponse
		mbWork      *mb.Work
	)

	var wg sync.WaitGroup

	// --- Spotify: GetTrack ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(isrcCh) // always unblock MB goroutine
		t0 := time.Now()
		ft, err := h.spotifyClient.Client.GetTrack(ctx, sid)
		if err != nil {
			l.Warnw("GetTrack failed", "error", err)
			return
		}
		l.Infow("GetTrack", "ms", time.Since(t0).Milliseconds())
		mu.Lock()
		fullTrack = ft
		mu.Unlock()
		if isrc, ok := ft.ExternalIDs["isrc"]; ok && isrc != "" {
			isrcCh <- isrc
		}
	}()

	// --- Spotify: GetAudioFeatures ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		f, err := h.spotifyClient.Client.GetAudioFeatures(ctx, sid)
		if err != nil {
			l.Warnw("GetAudioFeatures failed", "error", err)
			return
		}
		l.Infow("GetAudioFeatures", "ms", time.Since(t0).Milliseconds())
		mu.Lock()
		audioFeats = f
		mu.Unlock()
	}()

	// --- Spotify: GetAudioAnalysis ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		a, err := h.spotifyClient.Client.GetAudioAnalysis(ctx, sid)
		if err != nil {
			l.Warnw("GetAudioAnalysis failed", "error", err)
			return
		}
		l.Infow("GetAudioAnalysis", "ms", time.Since(t0).Milliseconds())
		mu.Lock()
		audioAnal = a
		mu.Unlock()
	}()

	// --- MusicBrainz: starts as soon as ISRC arrives ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		isrc, ok := <-isrcCh
		if !ok || isrc == "" {
			return
		}
		t0 := time.Now()
		searchResp, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(mb.SearchRecordingsByISRCRequest{
			ISRC: isrc,
		})
		if err != nil || searchResp.Count == 0 {
			l.Warnw("MB ISRC search failed or empty", "isrc", isrc, "error", err)
			return
		}
		mbid := searchResp.Recordings[0].ID
		l.Infow("MB ISRC resolved", "mbid", mbid, "ms", time.Since(t0).Milliseconds())

		t1 := time.Now()
		rec, err := h.musicbrainzClient.Client.GetRecording(mb.GetRecordingRequest{
			ID: mbid,
			Includes: []mb.Include{
				"artist-credits",
				"artist-rels",
				"genres",
				"isrcs",
				"work-rels",
				"url-rels",
			},
		})
		if err != nil {
			l.Warnw("MB GetRecording failed", "mbid", mbid, "error", err)
			return
		}
		l.Infow("MB GetRecording", "mbid", mbid, "ms", time.Since(t1).Milliseconds())

		mu.Lock()
		mbRecording = &rec
		mu.Unlock()

		// Work lookup — serial dep on recording, but concurrent with Spotify analysis
		work := h.getWorkFromRecordingWithLog(rec.Recording)
		if work != nil {
			mu.Lock()
			mbWork = work
			mu.Unlock()
		}
	}()

	wg.Wait()

	// --- Assemble track ---
	track := occipital.Track{
		SourceID: spotifyId,
		Source:   "SPOTIFY",
	}

	if fullTrack != nil {
		track.Name = fullTrack.Name
		track.Artist = util.GetFirstArtist(fullTrack.Artists)
		track.ReleaseDate = fullTrack.Album.ReleaseDate
		if len(fullTrack.Album.Images) > 0 {
			track.Image = fullTrack.Album.Images[0].URL
		}
		if isrc, ok := fullTrack.ExternalIDs["isrc"]; ok {
			track.ISRC = isrc
		}
		track.Popularity = int(fullTrack.Popularity)
		track.Links = []occipital.ExternalLink{
			{Type: "spotify", URL: fmt.Sprintf("https://open.spotify.com/track/%s", spotifyId)},
		}
	}

	if len(audioFeats) > 0 && audioFeats[0] != nil {
		af := audioFeats[0]
		track.Meta = &occipital.TrackMeta{
			DurationMs:    int(af.Duration),
			Key:           int(af.Key),
			Mode:          int(af.Mode),
			Tempo:         af.Tempo,
			TimeSignature: int(af.TimeSignature),
		}
		track.Features = &occipital.TrackFeatures{
			Acousticness:     af.Acousticness,
			Danceability:     af.Danceability,
			Energy:           af.Energy,
			Happiness:        af.Valence,
			Instrumentalness: af.Instrumentalness,
			Liveness:         af.Liveness,
			Loudness:         af.Loudness,
			Speechiness:      af.Speechiness,
		}
	}

	if audioAnal != nil {
		ta := occipital.TrackAnalysis{Duration: audioAnal.Track.Duration}
		for _, seg := range audioAnal.Segments {
			ta.Segments = append(ta.Segments, occipital.TrackAnalysisSegment{
				Start:         seg.Start,
				Duration:      seg.Duration,
				Confidence:    seg.Confidence,
				LoudnessStart: seg.LoudnessStart,
				LoudnessEnd:   seg.LoudnessEnd,
				LoudnessMax:   seg.LoudnessMax,
				Pitches:       seg.Pitches,
				Timbres:       seg.Timbre,
			})
		}
		track.Analysis = &ta
	}

	if mbRecording != nil {
		rec := mbRecording.Recording
		track.ID = rec.ID
		track.Instruments = getArtistInstrumentsForRecording(rec)
		track.ProductionCredits = getProductionCreditsForRecording(rec)
		track.Genres = getGenresForRecording(rec)

		for _, link := range getExternalLinksForRecording(rec) {
			if link.Type != "spotify" {
				track.Links = append(track.Links, link)
			}
		}
	}

	if mbWork != nil {
		track.SongCredits = getSongCreditsForWork(*mbWork)
	}

	return track
}

// --- Cache helpers ---

func (h *GetTrackV2Handler) getFromCache(ctx context.Context, spotifyId string) (*occipital.Track, bool) {
	if h.db == nil {
		return nil, false
	}
	doc, err := h.db.Collection(trackCacheCollection).Doc(spotifyId).Get(ctx)
	if err != nil {
		return nil, false
	}
	var cached cachedTrackDoc
	if err := doc.DataTo(&cached); err != nil {
		return nil, false
	}
	if time.Since(cached.CachedAt) > trackCacheTTL {
		h.log.Infow("Cache expired", "spotify_id", spotifyId)
		return nil, false
	}
	var track occipital.Track
	if err := json.Unmarshal([]byte(cached.TrackJSON), &track); err != nil {
		return nil, false
	}
	return &track, true
}

func (h *GetTrackV2Handler) saveToCache(ctx context.Context, spotifyId string, track *occipital.Track) {
	if h.db == nil {
		return
	}
	b, err := json.Marshal(track)
	if err != nil {
		h.log.Warnw("Failed to marshal track for cache", "error", err)
		return
	}
	_, err = h.db.Collection(trackCacheCollection).Doc(spotifyId).Set(ctx, cachedTrackDoc{
		TrackJSON: string(b),
		CachedAt:  time.Now(),
	})
	if err != nil {
		h.log.Warnw("Failed to write track cache", "spotify_id", spotifyId, "error", err)
		return
	}
	h.log.Infow("Track cached", "spotify_id", spotifyId)
}

func (h *GetTrackV2Handler) getWorkFromRecordingWithLog(rec mb.Recording) *mb.Work {
	for _, relation := range *rec.Relations {
		if relation.TargetType == "work" {
			work, err := h.musicbrainzClient.Client.GetWork(mb.GetWorkRequest{
				ID:       relation.Work.ID,
				Includes: []mb.Include{"artist-rels", "url-rels"},
			})
			if err != nil {
				h.log.Errorf("error fetching work: %v", err)
				return nil
			}
			return &work.Work
		}
	}
	return nil
}
