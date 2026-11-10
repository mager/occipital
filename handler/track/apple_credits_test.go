package track

import (
	"context"
	"encoding/json"
	"github.com/mager/occipital/musicbrainz"
	spot "github.com/zmb3/spotify/v2"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func appleTestTrack() *spot.FullTrack {
	var t spot.FullTrack
	t.Name = "Save My Love"
	t.Duration = 210404
	t.Artists = []spot.SimpleArtist{{Name: "Kygo"}, {Name: "Khalid"}, {Name: "Gryffin"}}
	t.Album.Name = "Save My Love"
	return &t
}

const appleSearchFixture = `{"results":[{"trackId":1876235280,"trackName":"Save My Love","artistName":"Kygo, Khalid & Gryffin","collectionName":"Save My Love - Single","trackTimeMillis":210404,"trackExplicitness":"notExplicit"}]}`

func TestAutomaticAppleCredits(t *testing.T) {
	page, err := os.ReadFile("testdata/apple-save-my-love.html")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &appleCreditClient{http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := string(page)
		if r.URL.Host == "itunes.apple.com" {
			body = appleSearchFixture
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}}
	credits, err := client.lookup(context.Background(), appleTestTrack())
	if err != nil || credits == nil || calls != 2 {
		t.Fatalf("lookup failed: %v calls=%d", err, calls)
	}
	if len(credits.songs) != 2 || len(credits.songs[0].Artists) != 5 || len(credits.production) != 6 || len(credits.performance) != 2 {
		t.Fatalf("incomplete roles: %+v", credits)
	}
	if credits.source.URL != "https://music.apple.com/us/song/save-my-love/1876235280" {
		t.Fatal("source lost")
	}
}
func TestAppleIdentityRejectsWrongEditions(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{
		{"artist", "Kygo, Khalid & Gryffin", "Kygo, Another & Gryffin"}, {"title", "Save My Love\"", "Save My Love (Live)\""}, {"album", "Save My Love - Single", "Another Album"}, {"duration", "210404", "230404"}, {"rating", "notExplicit", "explicit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectAppleTrack([]byte(strings.ReplaceAll(appleSearchFixture, tc.old, tc.new)), appleTestTrack())
			if err != nil || got != nil {
				t.Fatalf("wrong edition accepted: %+v %v", got, err)
			}
		})
	}
	ambiguous := strings.Replace(appleSearchFixture, "]}", ","+strings.Replace(strings.TrimSuffix(strings.TrimPrefix(appleSearchFixture, `{"results":[`), "]}"), "1876235280", "1876235281", 1)+"]}", 1)
	if got, err := selectAppleTrack([]byte(ambiguous), appleTestTrack()); err != nil || got != nil {
		t.Fatal("ambiguous match accepted", err)
	}
}
func TestApplePageIdentityAndSchema(t *testing.T) {
	page, _ := os.ReadFile("testdata/apple-save-my-love.html")
	match, _ := selectAppleTrack([]byte(appleSearchFixture), appleTestTrack())
	for _, body := range []string{`<html>unavailable</html>`, strings.ReplaceAll(string(page), "1876235280", "9999"), strings.ReplaceAll(string(page), "Gryffin", "Different"), strings.ReplaceAll(string(page), "Save My Love - Single", "Other Album")} {
		if got, err := parseAppleCredits([]byte(body), *match, appleTestTrack()); err == nil || got != nil {
			t.Fatal("unverified page accepted")
		}
	}
}

func TestAppleFailuresStayRetryable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limit", 429, `{}`}, {"server", 503, `{}`}, {"bad json", 200, `oops`}, {"oversize", 200, strings.Repeat("x", 2*1024*1024+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &appleCreditClient{http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}, Request: r}, nil
			})}}
			if _, err := c.lookup(context.Background(), appleTestTrack()); err == nil {
				t.Fatal("source failure treated as missing credits")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newAppleCreditClient()
	if _, err := c.lookup(ctx, appleTestTrack()); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestTrackCatalogKeepsAllArtistsWithoutAudioCalls(t *testing.T) {
	h := testTrackHandler(t, func(*http.Request) (int, string) { return 200, `{"recordings":[],"count":0}` }, 200)
	calls := 0
	h.spotifyClient.Client = spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.Contains(r.URL.Path, "/tracks/") {
			t.Error("unavailable audio API called", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"5ObyGDxNWH0Uuuk3NvC5r8","name":"Save My Love","duration_ms":210404,"explicit":true,"track_number":2,"disc_number":1,"artists":[{"name":"Kygo","id":"artist1"},{"name":"Khalid","id":"artist2"},{"name":"Gryffin","id":"artist3"}],"album":{"name":"Save My Love"}}`)), Header: http.Header{}, Request: r}, nil
	})})
	got, _, err := h.fetchParallel(context.Background(), "5ObyGDxNWH0Uuuk3NvC5r8")
	if err != nil || len(got.Artists) != 3 || !got.Explicit || got.TrackNumber != 2 || calls != 1 {
		t.Fatalf("lost catalog data or wasted calls: %+v calls=%d err=%v", got, calls, err)
	}
}

func TestAppleLiveCatalog(t *testing.T) {
	if os.Getenv("BEATBRAIN_LIVE_METADATA_TEST") != "1" {
		t.Skip("explicit live catalog check")
	}
	result, err := newAppleCreditClient().lookup(context.Background(), appleTestTrack())
	if err != nil || result == nil || len(result.songs) == 0 || len(result.production) == 0 {
		t.Fatalf("live credits missing: %+v %v", result, err)
	}
	t.Logf("Live source %s: %d songwriting, %d production, %d performance roles", result.source.URL, len(result.songs), len(result.production), len(result.performance))
}

func TestAllLinkedWorksContributeSongwriters(t *testing.T) {
	var rec musicbrainz.RecordingDetails
	err := json.Unmarshal([]byte(`{"relations":[{"target-type":"work","work":{"id":"work1","relations":[{"type":"composer","artist":{"id":"a","name":"First writer"}}]}},{"target-type":"work","work":{"id":"work2","relations":[{"type":"composer","artist":{"id":"b","name":"Second writer"}}]}}]}`), &rec)
	if err != nil {
		t.Fatal(err)
	}
	h := testTrackHandler(t, nil, 200)
	work, err := h.getWorkFromDetails(context.Background(), rec)
	if err != nil || work == nil {
		t.Fatal(err)
	}
	credits := getSongCreditsForWork(*work)
	if len(credits) != 1 || len(credits[0].Artists) != 2 {
		t.Fatalf("lost linked work credits: %+v", credits)
	}
}

func TestPartialCreditsRetryAfterAppleFailure(t *testing.T) {
	h := testTrackHandler(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "/recording/") {
			return 200, `{"id":"recording-id","title":"Manchild","relations":[{"type":"producer","target-type":"artist","artist":{"id":"producer-id","name":"Producer"}}]}`
		}
		return 200, `{"count":1,"recordings":[{"id":"recording-id","title":"Manchild","artist-credit":[{"artist":{"name":"Sabrina Carpenter"}}]}]}`
	}, 200)
	h.apple = &appleCreditClient{http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}, Request: r}, nil
	})}}
	got, cache, err := h.fetchParallel(context.Background(), "42UBPzRMh5yyz0EDPr6fr1")
	if err != nil || len(got.ProductionCredits) == 0 {
		t.Fatalf("partial credits lost: %+v %v", got, err)
	}
	if cache {
		t.Fatal("partial credits cached after Apple failed")
	}
}

func TestLinkedWorkRejectsWrongIdentity(t *testing.T) {
	h := testTrackHandler(t, func(*http.Request) (int, string) {
		return 200, `{"id":"wrong-work","relations":[{"type":"composer","artist":{"id":"a","name":"Wrong writer"}}]}`
	}, 200)
	var rec musicbrainz.RecordingDetails
	json.Unmarshal([]byte(`{"relations":[{"target-type":"work","work":{"id":"expected-work"}}]}`), &rec)
	work, err := h.getWorkFromDetails(context.Background(), rec)
	if err == nil || (work != nil && len(getSongCreditsForWork(*work)) > 0) {
		t.Fatal("wrong work identity accepted")
	}
}
