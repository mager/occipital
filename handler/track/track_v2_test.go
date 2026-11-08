package track

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testTrackHandler(t *testing.T, mbResponse func(*http.Request) (int, string), spotifyStatus int) *GetTrackV2Handler {
	t.Helper()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status, body := 403, `{}`
		if strings.Contains(r.URL.Host, "spotify") && strings.Contains(r.URL.Path, "/tracks/") {
			status, body = spotifyStatus, `{"id":"42UBPzRMh5yyz0EDPr6fr1","name":"Manchild","duration_ms":213000,"artists":[{"name":"Sabrina Carpenter"}],"external_ids":{"isrc":"USUM72504441"},"album":{"id":"album-id","name":"Man's Best Friend","release_date":"2025-08-29","release_date_precision":"day","album_type":"album","total_tracks":12,"images":[{"url":"https://example.com/art.jpg"}]}}`
		}
		if strings.Contains(r.URL.Host, "musicbrainz") && mbResponse != nil {
			status, body = mbResponse(r)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return NewGetTrackV2Handler(zap.NewNop().Sugar(), &spotify.SpotifyClient{Client: spot.New(&http.Client{Transport: transport})}, musicbrainz.ProvideMusicbrainz(), nil)
}

func TestTrackV2KeepsBasicMetadataWithoutAudioFeatures(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) { return 200, `{"count":0,"recordings":[]}` }, 200)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/track?spotifyId=42UBPzRMh5yyz0EDPr6fr1", nil))
	var body struct {
		Track struct {
			Meta *struct {
				Duration int `json:"duration_ms"`
				Key      int `json:"key"`
				Mode     int `json:"mode"`
			}
			Album *struct {
				Name string `json:"name"`
			}
			Features any
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Track.Meta == nil || body.Track.Meta.Duration != 213000 || body.Track.Meta.Key != -1 || body.Track.Meta.Mode != -1 {
		t.Fatalf("missing duration or false key: %s", w.Body.String())
	}
	if body.Track.Album == nil || body.Track.Album.Name != "Man's Best Friend" || body.Track.Features != nil {
		t.Fatalf("missing album or invented features: %s", w.Body.String())
	}
}

func TestTrackV2RejectsFailedSpotifyLookup(t *testing.T) {
	h := testTrackHandler(t, nil, 503)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/track?spotifyId=42UBPzRMh5yyz0EDPr6fr1", nil))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("got status %d; failed Spotify lookup must not be successful empty track", w.Code)
	}
}

func TestTrackV2RestoresReleasesAndHandlesMissingRelations(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "/recording/") {
			return 200, `{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"id":"artist-id","name":"Sabrina Carpenter"}}],"releases":[{"id":"release-id","title":"Man's Best Friend","status":"Official","date":"2025-08-29","artist-credit":[{"artist":{"id":"artist-id","name":"Sabrina Carpenter"}}]}]}`
		}
		return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"id":"artist-id","name":"Sabrina Carpenter"}}]}]}`
	}, 200)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/track?spotifyId=42UBPzRMh5yyz0EDPr6fr1", nil))
	var body GetTrackResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Track.Releases == nil || len(*body.Track.Releases) != 1 || (*body.Track.Releases)[0].Title != "Man's Best Friend" {
		t.Fatalf("missing releases: %s", w.Body.String())
	}
}

func TestCreditHelpersAcceptMissingRelations(t *testing.T) {
	if got := getSongCreditsForWork(mb.Work{}); len(got) != 0 {
		t.Fatal(got)
	}
	if got := getExternalLinksForRecording(mb.Recording{}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestTrackV2StopsOnCancellation(t *testing.T) {
	h := testTrackHandler(t, nil, 200)
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	started := time.Now()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/track?spotifyId=42UBPzRMh5yyz0EDPr6fr1", nil).WithContext(ctx))
	if time.Since(started) > time.Second {
		t.Fatal("request did not stop after cancellation")
	}
}

func TestRecordingFallbackRejectsOtherEditionsAndAmbiguity(t *testing.T) {
	var ft spot.FullTrack
	json.Unmarshal([]byte(`{"name":"Manchild","duration_ms":213000,"artists":[{"name":"Sabrina Carpenter"}],"album":{"name":"Man's Best Friend"}}`), &ft)
	var candidate musicbrainz.RecordingMatch
	json.Unmarshal([]byte(`{"id":"match","title":"Manchild","length":213001,"artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}`), &candidate)
	cases := []struct {
		name   string
		change func(*musicbrainz.RecordingMatch)
		want   string
	}{
		{"exact", func(*musicbrainz.RecordingMatch) {}, "match"},
		{"live title", func(r *musicbrainz.RecordingMatch) { r.Title = "Manchild (live)" }, ""},
		{"live disambiguation", func(r *musicbrainz.RecordingMatch) { r.Disambiguation = "live" }, ""},
		{"different length", func(r *musicbrainz.RecordingMatch) { r.Length = 190000 }, ""},
		{"unknown length", func(r *musicbrainz.RecordingMatch) { r.Length = 0 }, ""},
		{"wrong artist", func(r *musicbrainz.RecordingMatch) { r.ArtistCredits = nil }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := candidate
			tc.change(&r)
			if got := selectRecording([]musicbrainz.RecordingMatch{r}, &ft, false); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	other := candidate
	other.ID = "other"
	if got := selectRecording([]musicbrainz.RecordingMatch{candidate, other}, &ft, false); got != "" {
		t.Fatalf("accepted ambiguous match: %s", got)
	}
}

func TestTrackMetadataRejectsInvalidAudioValues(t *testing.T) {
	track := &occipital.Track{Meta: &occipital.TrackMeta{DurationMs: 213000, Key: 40, Mode: 5, Tempo: -3, TimeSignature: 50}, Features: &occipital.TrackFeatures{Energy: 2}}
	sanitizeTrackMetadata(track)
	if track.Meta.DurationMs != 213000 || track.Meta.Key != -1 || track.Meta.Mode != -1 || track.Meta.Tempo != 0 || track.Meta.TimeSignature != 0 || track.Features != nil {
		t.Fatalf("invalid audio values retained: %+v", track)
	}
}

func TestTrackCacheRejectsStaleSchemaAndEmptyResults(t *testing.T) {
	const valid = `{"source_id":"42UBPzRMh5yyz0EDPr6fr1","name":"Manchild","artist":"Sabrina Carpenter","meta":{"duration_ms":213000,"key":-1,"mode":-1}}`
	for _, tc := range []struct {
		name    string
		version int
		age     time.Duration
		body    string
		want    bool
	}{
		{"current", trackCacheVersion, time.Minute, valid, true},
		{"old schema", 0, time.Minute, valid, false},
		{"previous partial schema", 2, time.Minute, valid, false},
		{"previous featured artist mismatch", 3, time.Minute, valid, false},
		{"previous technical credits missing", 4, time.Minute, valid, false},
		{"expired incomplete", trackCacheVersion, 2 * time.Hour, valid, false},
		{"empty upstream", trackCacheVersion, time.Minute, `{"source_id":"42UBPzRMh5yyz0EDPr6fr1"}`, false},
		{"future timestamp", trackCacheVersion, -time.Hour, valid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := readCachedTrack(cachedTrackDoc{Version: tc.version, CachedAt: time.Now().Add(-tc.age), TrackJSON: tc.body})
			if ok != tc.want {
				t.Fatalf("got %v want %v", ok, tc.want)
			}
		})
	}
}

func TestTrackV2EmptyAudioFeaturePayloadDoesNotInventMeasurements(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) { return 200, `{"count":0,"recordings":[]}` }, 200)
	transport := http.DefaultTransport
	h.spotifyClient.Client = spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "audio-features") {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"audio_features":[{}]}`)), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/track?spotifyId=42UBPzRMh5yyz0EDPr6fr1", nil))
	var result GetTrackResponse
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Track.Meta == nil || result.Track.Meta.DurationMs != 213000 || result.Track.Meta.Key != -1 || result.Track.Features != nil {
		t.Fatalf("empty audio response replaced real metadata: %s", w.Body.String())
	}
}

func TestTrackV2FailedExpectedWorkLookupSkipsCache(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "/work/") {
			return 503, `{}`
		}
		if strings.Contains(r.URL.Path, "/recording/") {
			return 200, `{"id":"recording-id","title":"Manchild","relations":[{"target-type":"work","work":{"id":"work-id"}}]}`
		}
		return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}]}`
	}, 200)
	result, canCache, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
	if err != nil || result.ID != "recording-id" || result.Name != "Manchild" {
		t.Fatalf("partial track was lost: %+v, %v", result, err)
	}
	if canCache {
		t.Fatal("failed expected work lookup must stay retryable, even with a recording ID")
	}
}

func TestTrackV2RecordingWithoutWorkRemainsCacheable(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "/recording/") {
			return 200, `{"id":"recording-id","title":"Manchild"}`
		}
		return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}]}`
	}, 200)
	result, canCache, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
	if err != nil || result.ID != "recording-id" || !canCache {
		t.Fatalf("recording with no work should remain cacheable: %+v, %v, %v", result, canCache, err)
	}
}

func TestTrackV2FailedMusicBrainzLookupSkipsCache(t *testing.T) {
	for _, stage := range []string{"search", "recording"} {
		t.Run(stage, func(t *testing.T) {
			h := testTrackHandler(t, func(r *http.Request) (int, string) {
				if stage == "search" || strings.Contains(r.URL.Path, "/recording/") {
					return 503, `{}`
				}
				return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}]}`
			}, 200)
			result, canCache, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
			if err != nil || result.Name != "Manchild" {
				t.Fatalf("basic track was lost: %+v, %v", result, err)
			}
			if canCache {
				t.Fatal("failed MusicBrainz lookup must stay retryable")
			}
		})
	}
}

func TestTrackV2StableNoMatchRemainsCacheable(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) { return 200, `{"count":0,"recordings":[]}` }, 200)
	result, canCache, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
	if err != nil || result.Name != "Manchild" || result.ID != "" || !canCache {
		t.Fatalf("stable no-match was not cacheable: %+v, %v, %v", result, canCache, err)
	}
}
