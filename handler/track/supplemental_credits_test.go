package track

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mager/occipital/occipital"
	spot "github.com/zmb3/spotify/v2"
)

func reviewedTrack() occipital.Track {
	return occipital.Track{Source: "SPOTIFY", SourceID: "7oL7jAZkNyolF1H15Fhlm1", ISRC: "ES5022500431", Name: "Not a Pretty Girl", Artist: "Bb trickz", Meta: &occipital.TrackMeta{DurationMs: 105080}}
}

func cachedReviewedTrack(t *testing.T, track occipital.Track) *occipital.Track {
	t.Helper()
	b, err := json.Marshal(track)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := readCachedTrack(cachedTrackDoc{Version: trackCacheVersion, CachedAt: time.Now(), TrackJSON: string(b)})
	if !ok {
		t.Fatal("valid cached track rejected")
	}
	return got
}

func TestReviewedCreditsFillCachedTrackWithSourceAndNoInventedIDs(t *testing.T) {
	got := cachedReviewedTrack(t, reviewedTrack())
	wantSong := []*occipital.TrackSongCredit{
		{Credit: "composer", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}, {Name: "Clairo"}}},
		{Credit: "lyricist", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}}},
	}
	wantProduction := []*occipital.TrackProductionCredit{
		{Credit: "producer", Artists: []occipital.CreditArtist{{Name: "Clairo"}, {Name: "leodaleo"}}},
		{Credit: "mix", Artists: []occipital.CreditArtist{{Name: "leodaleo"}}},
		{Credit: "recording", Artists: []occipital.CreditArtist{{Name: "Julio Gómez"}}},
		{Credit: "mastering", Artists: []occipital.CreditArtist{{Name: "Joe LaPorta"}}},
	}
	if !reflect.DeepEqual(got.SongCredits, wantSong) || !reflect.DeepEqual(got.ProductionCredits, wantProduction) {
		t.Fatalf("missing reviewed credits: song=%+v production=%+v", got.SongCredits, got.ProductionCredits)
	}
	if got.ID != "" || !reflect.DeepEqual(got.Links, []occipital.ExternalLink{{Type: "youtube", URL: "https://www.youtube.com/watch?v=xQ6qgGwrFHg"}}) {
		t.Fatalf("invented recording ID or missing source: %+v", got)
	}
	if twice := cachedReviewedTrack(t, *got); !reflect.DeepEqual(twice, got) {
		t.Fatal("repeated enrichment added duplicate credits or links")
	}
}

func TestReviewedCreditsRequireVerifiedTrackIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*occipital.Track)
	}{
		{"source", func(v *occipital.Track) { v.Source = "OTHER" }},
		{"Spotify edition", func(v *occipital.Track) { v.SourceID = "other-id" }},
		{"ISRC", func(v *occipital.Track) { v.ISRC = "ES5022500432" }},
		{"title", func(v *occipital.Track) { v.Name += " (live)" }},
		{"artist", func(v *occipital.Track) { v.Artist = "Another Artist" }},
		{"duration", func(v *occipital.Track) { v.Meta.DurationMs = 110000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := reviewedTrack()
			tc.change(&input)
			got := cachedReviewedTrack(t, input)
			if len(got.SongCredits) != 0 || len(got.ProductionCredits) != 0 || len(got.Links) != 0 {
				t.Fatal("credits applied to unverified track")
			}
		})
	}
}

func TestReviewedCreditsPreserveExistingRoles(t *testing.T) {
	input := reviewedTrack()
	input.SongCredits = []*occipital.TrackSongCredit{{Credit: "composer", Artists: []occipital.CreditArtist{{ID: "verified-mbid", Name: "Existing Composer"}}}}
	input.ProductionCredits = []*occipital.TrackProductionCredit{{Credit: "producer", Artists: []occipital.CreditArtist{{ID: "other-mbid", Name: "Existing Producer"}}}}
	input.Links = []occipital.ExternalLink{{Type: "spotify", URL: "https://open.spotify.com/track/7oL7jAZkNyolF1H15Fhlm1"}}
	got := cachedReviewedTrack(t, input)
	if len(got.SongCredits) != 2 || len(got.ProductionCredits) != 4 || !reflect.DeepEqual(got.SongCredits[0], input.SongCredits[0]) || !reflect.DeepEqual(got.ProductionCredits[0], input.ProductionCredits[0]) || !reflect.DeepEqual(got.Links[0], input.Links[0]) {
		t.Fatal("existing credits replaced or missing roles not supplied")
	}
}

func TestReviewedCreditsSurviveMusicBrainzFailureOnFreshTrack(t *testing.T) {
	h := testTrackHandler(t, func(*http.Request) (int, string) { return 403, `{}` }, 200)
	transport := http.DefaultTransport
	h.spotifyClient.Client = spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/tracks/") {
			body := `{"id":"7oL7jAZkNyolF1H15Fhlm1","name":"Not a Pretty Girl","duration_ms":105080,"artists":[{"name":"Bb trickz"}],"external_ids":{"isrc":"ES5022500431"},"album":{"name":"Not a Pretty Girl","release_date":"2025-03-26"}}`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})})
	got, canCache, err := h.fetchParallel(context.Background(), "7oL7jAZkNyolF1H15Fhlm1")
	if err != nil || len(got.SongCredits) != 2 || len(got.ProductionCredits) != 4 || canCache {
		t.Fatalf("credits absent or failed lookup cached: %+v, cache=%v, err=%v", got, canCache, err)
	}
}

// Apple Music's credits for the exact 5816 Forest track, reviewed 2026-11-09.
func TestYodaReviewedCreditsAndIdentity(t *testing.T) {
	base := func() occipital.Track {
		return occipital.Track{Source: "SPOTIFY", SourceID: "5EQCcPs2QwFtT45eS1rQps", ISRC: "USZHR2400271", Name: "Yoda", Artist: "Tech N9ne", Meta: &occipital.TrackMeta{DurationMs: 210513}}
	}
	got := cachedReviewedTrack(t, base())
	if len(got.SongCredits) != 1 || got.SongCredits[0].Credit != "songwriter" || len(got.SongCredits[0].Artists) != 3 || len(got.ProductionCredits) != 5 || len(got.Instruments) != 2 {
		t.Fatalf("Yoda credits missing: %+v", got)
	}
	if got.ProductionCredits[0].Artists[0].Name != "JPZ" || got.Instruments[1].Artists[0].Name != "Donnie Yates" {
		t.Fatal("wrong roles")
	}
	if len(got.Links) != 1 || got.Links[0].URL != "https://music.apple.com/us/song/yoda/1813191831" {
		t.Fatal("missing credit source")
	}
	if twice := cachedReviewedTrack(t, *got); !reflect.DeepEqual(twice, got) {
		t.Fatal("duplicate reviewed credits")
	}
	for _, change := range []func(*occipital.Track){
		func(v *occipital.Track) { v.SourceID = "other" }, func(v *occipital.Track) { v.ISRC = "wrong" }, func(v *occipital.Track) { v.Artist = "Other" }, func(v *occipital.Track) { v.Name = "Yoda (live)" }, func(v *occipital.Track) { v.Meta.DurationMs++ },
	} {
		v := base()
		change(&v)
		applyReviewedCredits(&v)
		if len(v.SongCredits)+len(v.ProductionCredits)+len(v.Instruments) > 0 {
			t.Fatal("credits applied to another edition")
		}
	}
	existing := base()
	existing.Instruments = []*occipital.TrackInstrumentArtists{{Instrument: "vocals", Artists: []occipital.CreditArtist{{Name: "Existing credit"}}}}
	applyReviewedCredits(&existing)
	if existing.Instruments[0].Artists[0].Name != "Existing credit" {
		t.Fatal("existing performance role replaced")
	}
}

func TestLimestoneReviewedSongwriters(t *testing.T) {
	base := occipital.Track{Source: "SPOTIFY", SourceID: "2YT7gnUsicqZhmK3Fkb5L8", ISRC: "QM4DW1568120", Name: "Limestone", Artist: "Magic City Hippies", Meta: &occipital.TrackMeta{DurationMs: 209164}}
	got := cachedReviewedTrack(t, base)
	want := []occipital.CreditArtist{{Name: "Robert Joseph Hunter"}, {Name: "Patrick Scott Howard"}, {Name: "Johnn Coughlin"}}
	if len(got.SongCredits) != 1 || got.SongCredits[0].Credit != "songwriter" || !reflect.DeepEqual(got.SongCredits[0].Artists, want) {
		t.Fatalf("missing reviewed songwriters: %+v", got.SongCredits)
	}
	if len(got.Links) != 1 || got.Links[0].URL != "https://music.apple.com/us/song/limestone/1584203629" {
		t.Fatal("missing source")
	}
	if len(got.ProductionCredits) > 0 || len(got.Instruments) > 0 {
		t.Fatal("unverified roles added")
	}
	base.ISRC = "different"
	applyReviewedCredits(&base)
	if len(base.SongCredits) > 0 {
		t.Fatal("credits applied to another recording")
	}
}
