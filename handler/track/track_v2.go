package track

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
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

var spotifyTrackIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)

const (
	trackCacheCollection = "track_cache_v2"
	trackCacheTTL        = 7 * 24 * time.Hour
	trackCacheVersion    = 8
	trackRequestTimeout  = 8 * time.Second
)

type cachedTrackDoc struct {
	Version   int       `firestore:"version"`
	TrackJSON string    `firestore:"track_json"`
	CachedAt  time.Time `firestore:"cached_at"`
}

// GetTrackV2Handler is a fast, parallel, cached track handler.
type GetTrackV2Handler struct {
	apple             *appleCreditClient
	audioEnabled      bool
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
		apple:             newAppleCreditClient(),
		audioEnabled:      os.Getenv("OCCIPITAL_SPOTIFY_AUDIO_ENABLED") == "true",
		log:               log,
		spotifyClient:     spotifyClient,
		musicbrainzClient: musicbrainzClient,
		db:                db,
	}
}

// ServeHTTP handles GET /v2/track?spotifyId=xxx
func (h *GetTrackV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(r.Context(), trackRequestTimeout)
	defer cancel()
	q := r.URL.Query()
	spotifyId := q.Get("spotifyId")

	if !spotifyTrackIDPattern.MatchString(spotifyId) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "valid spotifyId required"})
		return
	}

	// --- Cache check ---
	if track, ok := h.getFromCache(ctx, spotifyId); ok {
		h.log.Infow("Cache hit", "spotify_id", spotifyId)
		json.NewEncoder(w).Encode(GetTrackResponse{Track: *track})
		return
	}

	h.log.Infow("Cache miss — fetching", "spotify_id", spotifyId)
	track, canCache, err := h.fetchParallel(ctx, spotifyId)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": "track unavailable"})
		return
	}

	// Fire-and-forget cache write
	if canCache && ctx.Err() == nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			h.saveToCache(cacheCtx, spotifyId, &track)
		}()
	}

	json.NewEncoder(w).Encode(GetTrackResponse{Track: track})
}

// fetchParallel fetches optional metadata within the request deadline:
//
//	t=0 → Spotify: GetTrack, GetAudioFeatures, GetAudioAnalysis (all concurrent)
//	t=track → MusicBrainz: ISRC or exact-name match → recording → work
func (h *GetTrackV2Handler) fetchParallel(ctx context.Context, spotifyId string) (occipital.Track, bool, error) {
	sid := spot.ID(spotifyId)
	fullTrack, err := h.spotifyClient.Client.GetTrack(ctx, sid)
	if err != nil {
		return occipital.Track{}, false, err
	}
	if fullTrack == nil {
		return occipital.Track{}, false, fmt.Errorf("Spotify returned no track")
	}
	var audioFeats []*spot.AudioFeatures
	var audioAnal *spot.AudioAnalysis
	var mbRecording *mb.Recording
	var mbWork *mb.Work
	var enrichmentErr, appleErr error
	var appleData *reviewedCredits
	var album *spotify.CatalogAlbum
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mbCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		rec, err := h.findRecording(mbCtx, fullTrack)
		enrichmentErr = err
		if rec == nil {
			return
		}
		mbRecording = &rec.Recording
		mbWork, enrichmentErr = h.getWorkFromDetails(mbCtx, *rec)
	}()
	if h.apple != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			appleCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			appleData, appleErr = h.apple.lookup(appleCtx, fullTrack)
			if appleErr != nil {
				h.log.Warnw("Apple credits unavailable", "spotify_id", spotifyId, "error", appleErr)
			}
		}()
	}
	if h.spotifyClient.HTTP != nil && fullTrack.Album.ID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			albumCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			album, _ = h.spotifyClient.GetCatalogAlbum(albumCtx, string(fullTrack.Album.ID))
		}()
	}
	if h.audioEnabled {
		wg.Add(2)
		go func() { defer wg.Done(); audioFeats, _ = h.spotifyClient.Client.GetAudioFeatures(ctx, sid) }()
		go func() { defer wg.Done(); audioAnal, _ = h.spotifyClient.Client.GetAudioAnalysis(ctx, sid) }()
	}
	wg.Wait()

	// --- Assemble track ---
	track := occipital.Track{
		SourceID: spotifyId,
		Source:   "SPOTIFY",
	}

	if fullTrack != nil {
		track.Name = fullTrack.Name
		track.Artist = util.GetFirstArtist(fullTrack.Artists)
		for _, artist := range fullTrack.Artists {
			track.Artists = append(track.Artists, occipital.TrackArtist{ID: string(artist.ID), Name: artist.Name, URL: "https://open.spotify.com/artist/" + string(artist.ID)})
		}
		track.Explicit = fullTrack.Explicit
		track.TrackNumber = int(fullTrack.TrackNumber)
		track.DiscNumber = int(fullTrack.DiscNumber)
		track.ReleaseDate = fullTrack.Album.ReleaseDate
		track.Album = &occipital.TrackAlbum{
			ID: string(fullTrack.Album.ID), Name: fullTrack.Album.Name,
			ReleaseDate: fullTrack.Album.ReleaseDate, ReleaseDatePrecision: fullTrack.Album.ReleaseDatePrecision,
			AlbumType: fullTrack.Album.AlbumType, URL: fmt.Sprintf("https://open.spotify.com/album/%s", fullTrack.Album.ID),
		}
		if album != nil {
			track.Album.Label = album.Label
			track.Album.TotalTracks = album.TotalTracks
			for _, copyright := range album.Copyrights {
				if copyright.Text != "" {
					track.Album.Copyrights = append(track.Album.Copyrights, copyright.Text)
				}
			}
		}
		track.Meta = &occipital.TrackMeta{DurationMs: int(fullTrack.Duration), Key: -1, Mode: -1}
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

	if len(audioFeats) > 0 && audioFeats[0] != nil && audioFeats[0].ID == sid && audioFeats[0].Duration > 0 {
		af := audioFeats[0]
		track.Meta = &occipital.TrackMeta{
			DurationMs:    int(fullTrack.Duration),
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
		rec := *mbRecording
		track.ID = rec.ID
		track.Instruments = getArtistInstrumentsForRecording(rec)
		track.ProductionCredits = getProductionCreditsForRecording(rec)
		track.Genres = getGenresForRecording(rec)
		track.Releases = getRecordingReleases(rec)
		track.Links = append(track.Links, occipital.ExternalLink{Type: "musicbrainz", URL: "https://musicbrainz.org/recording/" + rec.ID})

		for _, link := range getExternalLinksForRecording(rec) {
			if link.Type != "spotify" {
				track.Links = append(track.Links, link)
			}
		}
	}

	if mbWork != nil {
		track.SongCredits = getSongCreditsForWork(*mbWork)
	}

	track.MetadataStatus = "unmatched"
	if mbRecording != nil {
		track.MetadataStatus = "matched"
	}
	if enrichmentErr != nil {
		track.MetadataStatus = "unavailable"
	}
	sanitizeTrackMetadata(&track)
	if appleData != nil {
		mergeReviewedCredits(&track, *appleData)
	}
	applyReviewedCredits(&track)
	track.CreditsStatus = "missing"
	hasCredits := len(track.SongCredits)+len(track.ProductionCredits)+len(track.Instruments) > 0
	if hasCredits {
		track.CreditsStatus = "available"
	} else if enrichmentErr != nil || appleErr != nil {
		track.CreditsStatus = "unavailable"
	}
	// A failed metadata lookup must remain retryable. Return the partial track, but
	// do not store it for the normal seven-day recording cache lifetime.
	return track, enrichmentErr == nil && appleErr == nil, nil
}

// --- Cache helpers ---

func (h *GetTrackV2Handler) getFromCache(ctx context.Context, spotifyId string) (*occipital.Track, bool) {
	if h.db == nil {
		return nil, false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	doc, err := h.db.Collection(trackCacheCollection).Doc(spotifyId).Get(cacheCtx)
	if err != nil {
		return nil, false
	}
	var cached cachedTrackDoc
	if err := doc.DataTo(&cached); err != nil {
		return nil, false
	}
	return readCachedTrack(cached)
}

func readCachedTrack(cached cachedTrackDoc) (*occipital.Track, bool) {
	if cached.Version != trackCacheVersion || time.Since(cached.CachedAt) > trackCacheTTL || cached.CachedAt.After(time.Now()) {
		return nil, false
	}
	var track occipital.Track
	if err := json.Unmarshal([]byte(cached.TrackJSON), &track); err != nil {
		return nil, false
	}
	if !cacheableTrack(&track) || ((track.ID == "" || track.CreditsStatus == "missing") && time.Since(cached.CachedAt) > time.Hour) {
		return nil, false
	}
	applyReviewedCredits(&track)
	return &track, true
}

func (h *GetTrackV2Handler) saveToCache(ctx context.Context, spotifyId string, track *occipital.Track) {
	if h.db == nil || !cacheableTrack(track) {
		return
	}
	b, err := json.Marshal(track)
	if err != nil {
		h.log.Warnw("Failed to marshal track for cache", "error", err)
		return
	}
	_, err = h.db.Collection(trackCacheCollection).Doc(spotifyId).Set(ctx, cachedTrackDoc{
		Version:   trackCacheVersion,
		TrackJSON: string(b),
		CachedAt:  time.Now(),
	})
	if err != nil {
		h.log.Warnw("Failed to write track cache", "spotify_id", spotifyId, "error", err)
		return
	}
	h.log.Infow("Track cached", "spotify_id", spotifyId)
}

func cacheableTrack(track *occipital.Track) bool {
	return track != nil && track.SourceID != "" && track.Name != "" && track.Artist != "" && track.Meta != nil && track.Meta.DurationMs > 0
}

func (h *GetTrackV2Handler) getWorkFromDetails(ctx context.Context, rec musicbrainz.RecordingDetails) (*mb.Work, error) {
	if rec.Relations == nil {
		return nil, nil
	}
	relations := []mb.Relation{}
	seen := map[string]bool{}
	lookups := 0
	var lookupErr error
	for _, relation := range *rec.Relations {
		if relation.TargetType != "work" || relation.Work == nil || relation.Work.ID == "" || seen[relation.Work.ID] {
			continue
		}
		id := relation.Work.ID
		seen[id] = true
		work, ok := rec.Works[id]
		if !ok {
			if lookups >= 3 {
				lookupErr = fmt.Errorf("work lookup budget exceeded")
				continue
			}
			lookups++
			var err error
			work, err = h.musicbrainzClient.GetWorkContext(ctx, id)
			if err != nil {
				lookupErr = err
				continue
			}
		}
		if work.ID != id {
			lookupErr = fmt.Errorf("MusicBrainz returned an unexpected work")
			continue
		}
		if work.Relations != nil {
			relations = append(relations, (*work.Relations)...)
		}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	return &mb.Work{Relations: &relations}, lookupErr
}

func (h *GetTrackV2Handler) getWorkFromRecordingContext(ctx context.Context, rec mb.Recording) (*mb.Work, error) {
	if rec.Relations == nil {
		return nil, nil
	}
	for _, relation := range *rec.Relations {
		if relation.TargetType == "work" && relation.Work != nil && relation.Work.ID != "" {
			work, err := h.musicbrainzClient.GetWorkContext(ctx, relation.Work.ID)
			if err != nil {
				h.log.Warnw("MusicBrainz work unavailable", "error", err)
				return nil, err
			}
			if work.ID != relation.Work.ID {
				return nil, fmt.Errorf("MusicBrainz returned an unexpected work")
			}
			return &work, nil
		}
	}
	return nil, nil
}

// Missing or invalid measurements must not become a musical claim.
func sanitizeTrackMetadata(track *occipital.Track) {
	if meta := track.Meta; meta != nil {
		if meta.DurationMs < 0 || meta.DurationMs > 24*60*60*1000 {
			meta.DurationMs = 0
		}
		if meta.Key < -1 || meta.Key > 11 {
			meta.Key = -1
		}
		if meta.Mode < 0 || meta.Mode > 1 {
			meta.Mode = -1
		}
		if !finiteRange(meta.Tempo, 0, 1000) {
			meta.Tempo = 0
		}
		if meta.TimeSignature < 3 || meta.TimeSignature > 7 {
			meta.TimeSignature = 0
		}
	}
	if f := track.Features; f != nil {
		for _, value := range []float32{f.Acousticness, f.Danceability, f.Energy, f.Happiness, f.Instrumentalness, f.Liveness, f.Speechiness} {
			if !finiteRange(value, 0, 1) {
				track.Features = nil
				return
			}
		}
		if !finiteRange(f.Loudness, -100, 20) {
			track.Features = nil
		}
	}
}

func finiteRange(value, min, max float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) && value >= min && value <= max
}
