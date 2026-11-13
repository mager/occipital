package track

import (
	"context"
	"encoding/json"
	"github.com/mager/occipital/musicbrainz"
	spot "github.com/zmb3/spotify/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecordingConnectionsKeepDirectionAndRejectIncompleteLinks(t *testing.T) {
	var rec musicbrainz.RecordingDetails
	err := json.Unmarshal([]byte(`{"id":"11111111-1111-4111-8111-111111111111","relations":[
 {"target-type":"recording","type":"samples material","direction":"forward","recording":{"id":"22222222-2222-4222-8222-222222222222","title":"Source song","disambiguation":"album version","artist-credit":[{"name":"First","joinphrase":" & ","artist":{"name":"First Artist"}},{"artist":{"name":"Second"}}]}},
 {"target-type":"recording","type":"samples material","direction":"forward","recording":{"id":"22222222-2222-4222-8222-222222222222","title":"Source song"}},
 {"target-type":"recording","type":"remix","direction":"backward","recording":{"id":"33333333-3333-4333-8333-333333333333","title":"A remix"}},
 {"target-type":"recording","type":"edit","direction":"forward","recording":{"id":"../bad","title":"Invalid"}},
 {"target-type":"recording","type":"remix","direction":"sideways","recording":{"id":"44444444-4444-4444-8444-444444444444","title":"Invalid direction"}},
 {"target-type":"recording","type":"remaster","direction":"forward","recording":{"id":"55555555-5555-4555-8555-555555555555","title":"Deprecated"}},
 {"target-type":"recording","type":"samples material","direction":"backward","recording":{"id":"11111111-1111-4111-8111-111111111111","title":"Self"}},
 {"target-type":"recording","type":"edit","direction":"forward","recording":{"id":"66666666-6666-4666-8666-666666666666","title":"  "}},
 {"target-type":"artist","type":"remix","direction":"forward"}
 ]}`), &rec)
	if err != nil {
		t.Fatal(err)
	}
	got := getRecordingConnections(rec)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Type != "samples material" || got[0].Direction != "forward" || got[0].Artist != "First & Second" || got[0].Disambiguation != "album version" {
		t.Fatalf("sample lost context: %+v", got[0])
	}
	if got[1].Type != "remix" || got[1].Direction != "backward" || got[1].Artist != "" {
		t.Fatalf("remix changed direction: %+v", got[1])
	}
	if len(getRecordingConnections(musicbrainz.RecordingDetails{})) != 0 {
		t.Fatal("empty recording has connections")
	}
}

func TestTrackV2IncludesConnectionsInExistingLookup(t *testing.T) {
	calls := 0
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		calls++
		if strings.Contains(r.URL.Path, "/recording/") {
			if !strings.Contains(r.URL.Query().Get("inc"), "recording-rels") {
				t.Error("recording relationships not requested")
			}
			return 200, `{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}],"relations":[{"target-type":"recording","type":"remix","direction":"backward","recording":{"id":"33333333-3333-4333-8333-333333333333","title":"Manchild remix"}}]}`
		}
		return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}]}`
	}, 200)
	got, _, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
	if err != nil || len(got.Connections) != 1 || calls != 2 {
		t.Fatalf("connections missing or extra lookup: %+v calls=%d err=%v", got.Connections, calls, err)
	}
}

func TestStudioDetailsPreservePlaceAndDatePrecision(t *testing.T) {
	var rec musicbrainz.RecordingDetails
	json.Unmarshal([]byte(`{"relations":[
 {"target-type":"place","type":"recorded at","direction":"backward","begin":"2000-03","end":"2000-04","place":{"id":"22222222-2222-4222-8222-222222222222","name":"A studio","area":{"name":"Chicago"}}},
 {"target-type":"place","type":"mixed at","direction":"backward","begin":"2000-02-31","place":{"id":"22222222-2222-4222-8222-222222222222","name":"A studio"}},
 {"target-type":"place","type":"recorded at","direction":"backward","begin":"2000-03","end":"2000-04","place":{"id":"22222222-2222-4222-8222-222222222222","name":"A studio"}},
 {"target-type":"place","type":"recorded at","direction":"forward","place":{"id":"33333333-3333-4333-8333-333333333333","name":"Wrong direction"}},
 {"target-type":"place","type":"video shot at","direction":"backward","place":{"id":"44444444-4444-4444-8444-444444444444","name":"Film set"}}
 ]}`), &rec)
	got := getRecordingStudios(rec)
	if len(got) != 2 || got[0].Name != "A studio" || got[0].Area != "Chicago" || got[0].Begin != "2000-03" || got[0].End != "2000-04" || got[1].Begin != "" {
		t.Fatalf("bad studio data: %+v", got)
	}
}

func TestDirectRecordingAcceptsCanonicalIDWithoutImageFanout(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		return 200, `{"id":"33333333-3333-4333-8333-333333333333","title":"Canonical recording","length":123000,"artist-credit":[{"artist":{"id":"artist","name":"Artist"}}],"releases":[{"id":"release","title":"Release","status":"Official","artist-credit":[{"artist":{"id":"artist","name":"Artist"}}]}],"relations":[]}`
	}, 200)
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "musicbrainz.org" {
			t.Errorf("unexpected additional request: %s", r.URL.Host)
		}
		return original.RoundTrip(r)
	})
	direct := NewGetTrackHandler(h.log, h.spotifyClient, h.musicbrainzClient)
	w := httptest.NewRecorder()
	direct.ServeHTTP(w, httptest.NewRequest("GET", "/track?mbid=22222222-2222-4222-8222-222222222222", nil))
	var body GetTrackResponse
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Track.ID != "33333333-3333-4333-8333-333333333333" || body.Track.Meta.DurationMs != 123000 || body.Track.Image == "" {
		t.Fatalf("canonical lookup failed: %d %s", w.Code, w.Body.String())
	}
}

func TestDirectRecordingStopsOnCancellation(t *testing.T) {
	h := testTrackHandler(t, nil, 200)
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	NewGetTrackHandler(h.log, h.spotifyClient, h.musicbrainzClient).ServeHTTP(w, httptest.NewRequest("GET", "/track?mbid=22222222-2222-4222-8222-222222222222", nil).WithContext(ctx))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d", w.Code)
	}
}

func TestDirectRecordingKeepsUsableSpotifyArt(t *testing.T) {
	h := testTrackHandler(t, nil, 200)
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"33333333-3333-4333-8333-333333333333","title":"Manchild","length":213000,"isrcs":["USUM72504441"],"artist-credit":[{"artist":{"id":"artist","name":"Sabrina Carpenter"}}],"releases":[{"id":"release","title":"Release","status":"Official","artist-credit":[{"artist":{"id":"artist","name":"Sabrina Carpenter"}}]}]}`
		if strings.Contains(r.URL.Host, "spotify") {
			body = `{"tracks":{"items":[{"id":"42UBPzRMh5yyz0EDPr6fr1","name":"Manchild","duration_ms":213000,"external_ids":{"isrc":"USUM72504441"},"artists":[{"name":"Sabrina Carpenter"}],"album":{"images":[{"url":"https://example.com/known-cover.jpg"}]}}]}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	http.DefaultTransport = transport
	h.spotifyClient.Client = spot.New(&http.Client{Transport: transport})
	w := httptest.NewRecorder()
	NewGetTrackHandler(h.log, h.spotifyClient, h.musicbrainzClient).ServeHTTP(w, httptest.NewRequest("GET", "/track?mbid=33333333-3333-4333-8333-333333333333", nil))
	var body GetTrackResponse
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Track.Image != "https://example.com/known-cover.jpg" || body.Track.SourceID != "42UBPzRMh5yyz0EDPr6fr1" {
		t.Fatalf("catalog cover lost: %d %s", w.Code, w.Body.String())
	}
}
