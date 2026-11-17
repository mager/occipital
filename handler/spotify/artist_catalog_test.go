package spotify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type catalogFixture struct {
	albums   []map[string]any
	tracks   map[string][]map[string]any
	override func(http.ResponseWriter, *http.Request) bool
	mu       sync.Mutex
	requests []string
}

func catalogID(n int) string { return fmt.Sprintf("%022d", n) }

func catalogAlbum(n int) map[string]any {
	return map[string]any{"id": catalogID(n), "name": "G I R L", "album_type": "album", "album_group": "album", "release_date": "2014-03-03", "images": []map[string]string{{"url": "https://i.scdn.co/image/girl"}}, "external_urls": map[string]string{"spotify": "https://attacker.test"}}
}

func catalogTrack(n int, credited string) map[string]any {
	return map[string]any{"id": catalogID(n), "name": fmt.Sprintf("Song %d", n), "artists": []map[string]string{{"id": credited, "name": "Pharrell Williams"}}}
}

func (f *catalogFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.URL.RequestURI())
		f.mu.Unlock()
		if f.override != nil && f.override(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/artists/"+artistID:
			io.WriteString(w, artistJSON)
		case r.URL.Path == "/search":
			// The search source can claim it is complete with only five songs.
			items := make([]map[string]any, 5)
			for i := range items {
				items[i] = catalogTrack(i+1, artistID)
			}
			json.NewEncoder(w).Encode(map[string]any{"tracks": map[string]any{"total": 5, "next": nil, "items": items}})
		case r.URL.Path == "/artists/"+artistID+"/albums":
			writeCatalogPage(t, w, r, f.albums)
		case strings.HasPrefix(r.URL.Path, "/albums/") && strings.HasSuffix(r.URL.Path, "/tracks"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/albums/"), "/tracks")
			if r.URL.Query().Get("limit") != "50" || r.URL.Query().Get("market") != "US" {
				t.Errorf("unbounded or marketless tracks request: %s", r.URL)
			}
			writeCatalogPage(t, w, r, f.tracks[id])
		default:
			t.Errorf("unexpected provider endpoint %s", r.URL)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeCatalogPage(t *testing.T, w http.ResponseWriter, r *http.Request, items []map[string]any) {
	t.Helper()
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 20
	}
	start := min(offset, len(items))
	end := min(start+limit, len(items))
	page := items[start:end]
	if page == nil {
		page = []map[string]any{}
	}
	var next any
	if end < len(items) {
		next = "https://api.spotify.com/next"
	}
	json.NewEncoder(w).Encode(map[string]any{"offset": offset, "limit": limit, "total": len(items), "next": next, "items": page})
}

type catalogResponse struct {
	Artist ArtistDetails `json:"artist"`
	Tracks []struct {
		ID          string  `json:"id"`
		Thumb       *string `json:"thumb"`
		AlbumID     string  `json:"album_id"`
		AlbumName   string  `json:"album_name"`
		ReleaseDate string  `json:"release_date"`
	} `json:"tracks"`
	Albums []struct {
		ID, URL    string
		Image      *string
		AlbumGroup string `json:"album_group"`
	} `json:"albums"`
	NextCursor        *string  `json:"next_cursor"`
	NextReleaseOffset *int     `json:"next_release_offset"`
	NextOffset        *int     `json:"next_offset"`
	ReleaseTotal      int      `json:"release_total"`
	ScannedReleases   int      `json:"scanned_releases"`
	Unavailable       []string `json:"unavailable"`
}

func readCatalog(t *testing.T, h http.Handler, query string) catalogResponse {
	t.Helper()
	w := artistRequest(h, "spotifyId="+artistID+query)
	var result catalogResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
	if result.Tracks == nil || result.Albums == nil || result.Unavailable == nil {
		t.Fatalf("missing bounded arrays: %s", w.Body.String())
	}
	return result
}

func TestArtistCatalogIncludesWholeAlbumWhenSearchStopsAtFive(t *testing.T) {
	f := &catalogFixture{albums: []map[string]any{catalogAlbum(100)}, tracks: map[string][]map[string]any{}}
	for i := 1; i <= 11; i++ {
		f.tracks[catalogID(100)] = append(f.tracks[catalogID(100)], catalogTrack(i, artistID))
	}
	b := readCatalog(t, artistHandlerForServer(f.serve(t)), "")
	if len(b.Tracks) != 11 {
		t.Fatalf("full album has 11 songs, got %d", len(b.Tracks))
	}
	if b.Artist.ID != artistID || b.Artist.URL != "https://open.spotify.com/artist/"+artistID {
		t.Errorf("wrong artist: %+v", b.Artist)
	}
	for _, track := range b.Tracks {
		if track.AlbumID != catalogID(100) || track.AlbumName != "G I R L" || track.ReleaseDate != "2014-03-03" || track.Thumb == nil || *track.Thumb != "https://i.scdn.co/image/girl" {
			t.Errorf("missing album context: %+v", track)
		}
	}
	if len(b.Albums) != 1 || b.Albums[0].URL != "https://open.spotify.com/album/"+catalogID(100) || b.Albums[0].AlbumGroup != "album" {
		t.Errorf("wrong release: %+v", b.Albums)
	}
	if b.NextOffset != nil || b.NextCursor != nil || b.NextReleaseOffset != nil || b.ReleaseTotal != 1 || b.ScannedReleases != 1 {
		t.Errorf("wrong final page: %+v", b)
	}
	for _, request := range f.requests {
		if strings.HasPrefix(request, "/search") {
			t.Error("artist catalog used the incomplete search source")
		}
	}
}

func TestArtistCatalogResumesAtRawTrackAfterFilteredAndDuplicateItems(t *testing.T) {
	tracks := []map[string]any{catalogTrack(900, "0000000000000000000000"), catalogTrack(901, artistID), catalogTrack(1, artistID), catalogTrack(1, artistID)}
	tracks[1]["id"] = "invalid"
	for i := 2; i <= 25; i++ {
		tracks = append(tracks, catalogTrack(i, artistID))
	}
	f := &catalogFixture{albums: []map[string]any{catalogAlbum(100)}, tracks: map[string][]map[string]any{catalogID(100): tracks}}
	h := artistHandlerForServer(f.serve(t))
	first := readCatalog(t, h, "&section=songs")
	if len(first.Tracks) != 24 || first.NextCursor == nil || *first.NextCursor != "0.27" || first.ScannedReleases != 0 {
		t.Fatalf("wrong first batch: %+v", first)
	}
	second := readCatalog(t, h, "&section=songs&cursor="+*first.NextCursor)
	if len(second.Tracks) != 1 || second.Tracks[0].ID != catalogID(25) || second.NextCursor != nil || second.ScannedReleases != 1 {
		t.Errorf("mid-album continuation skipped or repeated songs: %+v", second)
	}
}

func TestArtistCatalogContinuesWithinAlbumAfterFiftyExcludedTracks(t *testing.T) {
	tracks := []map[string]any{}
	for i := 1; i <= 50; i++ {
		tracks = append(tracks, catalogTrack(i, "0000000000000000000000"))
	}
	tracks = append(tracks, catalogTrack(51, artistID))
	f := &catalogFixture{albums: []map[string]any{catalogAlbum(100)}, tracks: map[string][]map[string]any{catalogID(100): tracks}}
	h := artistHandlerForServer(f.serve(t))
	first := readCatalog(t, h, "&section=songs")
	if len(first.Tracks) != 0 || first.NextCursor == nil || *first.NextCursor != "0.50" || first.ScannedReleases != 0 {
		t.Fatalf("wrong 50-track continuation: %+v", first)
	}
	second := readCatalog(t, h, "&section=songs&cursor=0.50")
	if len(second.Tracks) != 1 || second.Tracks[0].ID != catalogID(51) || second.NextCursor != nil || second.ScannedReleases != 1 {
		t.Errorf("lost late album track: %+v", second)
	}
}

func TestArtistCatalogBoundsEmptyReleaseScanAndFiltersCompilationCredits(t *testing.T) {
	albums := []map[string]any{}
	for i := 100; i <= 103; i++ {
		albums = append(albums, catalogAlbum(i))
	}
	albums[3]["album_type"] = "compilation"
	albums[3]["album_group"] = "appears_on"
	f := &catalogFixture{albums: albums, tracks: map[string][]map[string]any{catalogID(103): {catalogTrack(1, "0000000000000000000000"), catalogTrack(2, artistID), catalogTrack(2, artistID)}}}
	h := artistHandlerForServer(f.serve(t))
	first := readCatalog(t, h, "&section=songs")
	if len(first.Tracks) != 0 || first.NextCursor == nil || *first.NextCursor != "3.0" || first.ScannedReleases != 3 {
		t.Fatalf("empty releases did not make bounded progress: %+v", first)
	}
	second := readCatalog(t, h, "&section=songs&cursor=3.0")
	if len(second.Tracks) != 1 || second.Tracks[0].ID != catalogID(2) || second.NextCursor != nil || second.ScannedReleases != 4 {
		t.Errorf("wrong compilation credits: %+v", second)
	}
}

func TestArtistCatalogPagesAll770ReleasesIndependently(t *testing.T) {
	f := &catalogFixture{tracks: map[string][]map[string]any{}}
	for i := 0; i < 770; i++ {
		f.albums = append(f.albums, catalogAlbum(100+i))
	}
	h := artistHandlerForServer(f.serve(t))
	for _, tc := range []struct{ offset, count, next int }{{0, 24, 24}, {24, 24, 48}, {744, 24, 768}, {768, 2, -1}, {800, 0, -1}} {
		b := readCatalog(t, h, fmt.Sprintf("&section=releases&offset=%d", tc.offset))
		if len(b.Albums) != tc.count || b.ReleaseTotal != 770 || len(b.Tracks) != 0 || b.NextCursor != nil || b.NextOffset != nil {
			t.Errorf("offset %d: %+v", tc.offset, b)
		}
		if tc.next < 0 {
			if b.NextReleaseOffset != nil {
				t.Errorf("past final releases: %+v", b)
			}
		} else if b.NextReleaseOffset == nil || *b.NextReleaseOffset != tc.next {
			t.Errorf("release continuation at %d: %+v", tc.offset, b)
		}
	}
	for _, request := range f.requests {
		if strings.Contains(request, "/albums?") && (!strings.Contains(request, "include_groups=album%2Csingle%2Cappears_on%2Ccompilation") || !strings.Contains(request, "market=US")) {
			t.Errorf("missing artist release groups or market: %s", request)
		}
		if strings.Contains(request, "/tracks?") {
			t.Error("release-only request loaded tracks")
		}
	}
}

func TestArtistCatalogPartialTrackFailureRetriesExactPositionWithoutCaching(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var identityCalls atomic.Int32
	f := &catalogFixture{albums: []map[string]any{catalogAlbum(100), catalogAlbum(101)}, tracks: map[string][]map[string]any{catalogID(100): {catalogTrack(1, artistID)}, catalogID(101): {catalogTrack(2, artistID)}}}
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/artists/"+artistID {
			identityCalls.Add(1)
		}
		if r.URL.Path == "/albums/"+catalogID(101)+"/tracks" && fail.Load() {
			w.WriteHeader(500)
			io.WriteString(w, `{"error":{"status":500,"message":"private"}}`)
			return true
		}
		return false
	}
	h := artistHandlerForServer(f.serve(t))
	for i := 0; i < 2; i++ {
		b := readCatalog(t, h, "&section=songs")
		if len(b.Tracks) != 1 || b.NextCursor == nil || *b.NextCursor != "1.0" || b.ScannedReleases != 1 || strings.Join(b.Unavailable, ",") != "tracks" {
			t.Fatalf("partial batch lost data or skipped failed release: %+v", b)
		}
	}
	if identityCalls.Load() != 2 {
		t.Error("partial batch was cached")
	}
	fail.Store(false)
	b := readCatalog(t, h, "&section=songs&cursor=1.0")
	if len(b.Tracks) != 1 || b.Tracks[0].ID != catalogID(2) || b.NextCursor != nil || len(b.Unavailable) != 0 {
		t.Errorf("retry skipped failed position: %+v", b)
	}
}

func TestArtistCatalogReleaseFailureKeepsRetryOffset(t *testing.T) {
	f := &catalogFixture{override: func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/albums") {
			w.WriteHeader(500)
			io.WriteString(w, `{"error":{"status":500,"message":"private"}}`)
			return true
		}
		return false
	}}
	b := readCatalog(t, artistHandlerForServer(f.serve(t)), "&section=releases&offset=744")
	if b.NextReleaseOffset == nil || *b.NextReleaseOffset != 744 || strings.Join(b.Unavailable, ",") != "albums" || len(b.Tracks) != 0 {
		t.Errorf("release failure cannot retry: %+v", b)
	}
}

func TestArtistCatalogRejectsInvalidNavigationBeforeProvider(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	h := artistHandlerForServer(server)
	for _, suffix := range []string{"&section=bad", "&section=", "&section=songs&section=releases", "&cursor=", "&cursor=0", "&cursor=0.0.0", "&cursor=-1.0", "&cursor=01.0", "&cursor=0.01", "&cursor=1000001.0", "&cursor=0.1000001", "&cursor=0.0&cursor=1.0", "&offset=1000001", "&section=all&offset=1", "&section=all&cursor=1.0", "&section=songs&offset=1", "&section=releases&cursor=1.0", "&section=releases&offset=01"} {
		w := artistRequest(h, "spotifyId="+artistID+suffix)
		if w.Code != 400 || !json.Valid(w.Body.Bytes()) {
			t.Errorf("invalid query %q: %d %s", suffix, w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Errorf("invalid queries reached provider %d times", calls.Load())
	}
}

func TestArtistCatalogCacheSeparatesSectionCursorAndOffset(t *testing.T) {
	var identities atomic.Int32
	f := &catalogFixture{albums: []map[string]any{catalogAlbum(100), catalogAlbum(101)}, tracks: map[string][]map[string]any{}, override: func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/artists/"+artistID {
			identities.Add(1)
		}
		return false
	}}
	h := artistHandlerForServer(f.serve(t))
	queries := []string{"", "&section=songs", "&section=songs&cursor=1.0", "&section=releases", "&section=releases&offset=1"}
	for _, query := range queries {
		first := readCatalog(t, h, query)
		second := readCatalog(t, h, query)
		if len(first.Albums) != len(second.Albums) || len(first.Tracks) != len(second.Tracks) {
			t.Error("cached response changed")
		}
	}
	if identities.Load() != 5 {
		t.Errorf("cache key collision or missing cache: identity loads=%d", identities.Load())
	}
}

func TestArtistCatalogAllKeepsSuccessfulSectionWhenOtherSectionFails(t *testing.T) {
	for _, failed := range []string{"tracks", "albums"} {
		t.Run(failed, func(t *testing.T) {
			var identities atomic.Int32
			f := &catalogFixture{albums: []map[string]any{catalogAlbum(100)}, tracks: map[string][]map[string]any{catalogID(100): {catalogTrack(1, artistID)}}}
			f.override = func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/artists/"+artistID {
					identities.Add(1)
				}
				fail := (failed == "tracks" && strings.HasSuffix(r.URL.Path, "/tracks")) || (failed == "albums" && strings.HasSuffix(r.URL.Path, "/albums") && r.URL.Query().Get("limit") == "24")
				if fail {
					w.WriteHeader(500)
					io.WriteString(w, `{"error":{"status":500,"message":"private"}}`)
					return true
				}
				return false
			}
			h := artistHandlerForServer(f.serve(t))
			for i := 0; i < 2; i++ {
				b := readCatalog(t, h, "")
				if strings.Join(b.Unavailable, ",") != failed {
					t.Errorf("wrong failed section: %+v", b)
				}
				if failed == "tracks" && (len(b.Albums) != 1 || b.NextCursor == nil || *b.NextCursor != "0.0") {
					t.Errorf("successful releases lost: %+v", b)
				}
				if failed == "albums" && (len(b.Tracks) != 1 || b.NextReleaseOffset == nil || *b.NextReleaseOffset != 0) {
					t.Errorf("successful songs lost: %+v", b)
				}
			}
			if identities.Load() != 2 {
				t.Error("partial initial response was cached")
			}
		})
	}
}
