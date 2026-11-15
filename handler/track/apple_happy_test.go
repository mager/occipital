package track

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	spot "github.com/zmb3/spotify/v2"
)

// Spotify and the Apple fixtures were checked against the public catalogs on
// 2026-11-15. Apple uses a parenthetical soundtrack label for Spotify's dash label.
func happyTestTrack() *spot.FullTrack {
	var track spot.FullTrack
	track.ID = "60nZcImufyMA1MKQY3dcCH"
	track.Name = `Happy - From "Despicable Me 2"`
	track.Duration = 233306
	track.Artists = []spot.SimpleArtist{{Name: "Pharrell Williams"}}
	track.Album.Name = "G I R L"
	track.Album.AlbumType = "album"
	return &track
}

func happyAppleFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestHappyAppleCredits(t *testing.T) {
	search := happyAppleFixture(t, "apple-happy-search.json")
	page := happyAppleFixture(t, "apple-happy.html")
	client := &appleCreditClient{http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case r.URL.Host == "itunes.apple.com" && r.URL.Path == "/search":
			if r.URL.Query().Get("term") != `Pharrell Williams Happy - From "Despicable Me 2"` {
				t.Fatalf("unexpected catalog query: %s", r.URL)
			}
			body = search
		case r.URL.Host == "music.apple.com" && r.URL.Path == "/us/song/863835363":
			body = page
		default:
			t.Fatalf("unexpected credit request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})}}
	credits, err := client.lookup(context.Background(), happyTestTrack())
	if err != nil || credits == nil {
		t.Fatalf("Happy credits missing: %+v %v", credits, err)
	}
	if len(credits.songs) != 2 || len(credits.production) != 6 || len(credits.performance) != 3 {
		t.Fatalf("incomplete Happy credits: %+v", credits)
	}
	for _, want := range []string{"song:songwriter:Pharrell Williams", "production:producer:Pharrell Williams", "production:mix:Leslie Brathwaite", "production:mastering:Bob Ludwig", "performance:background vocals:Rhea Dummett"} {
		if !strings.Contains(creditSignature(credits), want) {
			t.Errorf("Happy credit missing: %s", want)
		}
	}
	if credits.source.URL != "https://music.apple.com/us/song/happy-from-despicable-me-2/863835363" {
		t.Fatalf("wrong credit source: %s", credits.source.URL)
	}
}

func TestAppleSoundtrackTitlePreservesIdentity(t *testing.T) {
	base := appleCatalogTrack{ID: 863835363, Name: `Happy (From "Despicable Me 2")`, Artist: "Pharrell Williams", Album: "G I R L", Duration: 233307, Rating: "notExplicit"}
	for _, tc := range []struct {
		name   string
		change func(*appleCatalogTrack)
	}{
		{"wrong artist", func(c *appleCatalogTrack) { c.Artist = "Another Artist" }},
		{"wrong album edition", func(c *appleCatalogTrack) { c.Album = "G I R L (Deluxe)" }},
		{"wrong duration", func(c *appleCatalogTrack) { c.Duration = 234307 }},
		{"wrong explicitness", func(c *appleCatalogTrack) { c.Rating = "explicit" }},
		{"wrong soundtrack", func(c *appleCatalogTrack) { c.Name = `Happy (From "Despicable Me 3")` }},
		{"missing soundtrack", func(c *appleCatalogTrack) { c.Name = "Happy" }},
		{"remix suffix", func(c *appleCatalogTrack) { c.Name += " - Remix" }},
		{"live suffix", func(c *appleCatalogTrack) { c.Name += " (Live)" }},
		{"unquoted soundtrack", func(c *appleCatalogTrack) { c.Name = "Happy (From Despicable Me 2)" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.change(&candidate)
			body, err := json.Marshal(map[string]any{"results": []appleCatalogTrack{candidate}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := selectAppleTrack(body, happyTestTrack())
			if err != nil || got != nil {
				t.Fatalf("wrong soundtrack edition accepted: %+v %v", got, err)
			}
		})
	}
	other := base
	other.ID++
	other.Name = happyTestTrack().Name
	body, err := json.Marshal(map[string]any{"results": []appleCatalogTrack{base, other}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := selectAppleTrack(body, happyTestTrack()); err != nil || got != nil {
		t.Fatalf("ambiguous soundtrack IDs accepted: %+v %v", got, err)
	}
}

func TestAppleSoundtrackPagePreservesIdentity(t *testing.T) {
	page := happyAppleFixture(t, "apple-happy.html")
	match := appleCatalogTrack{ID: 863835363, Name: `Happy (From "Despicable Me 2")`, Album: "G I R L"}
	for _, tc := range []struct{ name, old, new string }{
		{"wrong soundtrack", "Despicable Me 2", "Despicable Me 3"},
		{"wrong artist", "Pharrell Williams", "Another Artist"},
		{"wrong album edition", "G I R L", "G I R L (Deluxe)"},
		{"wrong page ID", "863835363", "999999999"},
		{"remix", `Happy (From \"Despicable Me 2\")`, `Happy (From \"Despicable Me 2\") - Remix`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(string(page), tc.old, tc.new)
			if body == string(page) {
				t.Fatal("fixture mutation did not change page")
			}
			if got, err := parseAppleCredits([]byte(body), match, happyTestTrack()); err == nil || got != nil {
				t.Fatalf("unverified soundtrack page accepted: %+v %v", got, err)
			}
		})
	}
}

func TestHappyLiveAppleCredits(t *testing.T) {
	if os.Getenv("BEATBRAIN_LIVE_METADATA_TEST") != "1" {
		t.Skip("explicit live source check")
	}
	credits, err := newAppleCreditClient().lookup(context.Background(), happyTestTrack())
	if err != nil || credits == nil || len(credits.songs) == 0 || len(credits.production) == 0 {
		t.Fatalf("live Happy credits missing: %+v %v", credits, err)
	}
	t.Logf("Source %s: %d songwriting, %d production, %d performance roles", credits.source.URL, len(credits.songs), len(credits.production), len(credits.performance))
}
