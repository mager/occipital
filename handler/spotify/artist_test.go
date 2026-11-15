package spotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	client "github.com/mager/occipital/spotify"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

const artistID = "2RdwBSPQiwcmiDo9kixcl8"
const artistJSON = `{"id":"2RdwBSPQiwcmiDo9kixcl8","name":"Pharrell Williams","genres":["hip hop"],"images":[{"url":"https://i.scdn.co/image/artist"}],"external_urls":{"spotify":"https://attacker.test"}}`
const artistTracksJSON = `{"tracks":{"total":25,"next":"https://api.spotify.com/v1/search?offset=10","items":[{"id":"happy","name":"Happy","artists":[{"id":"2RdwBSPQiwcmiDo9kixcl8","name":"Pharrell Williams"}],"album":{"images":[{"url":"https://i.scdn.co/image/happy","height":300,"width":300}]}},{"id":"wrong","name":"Wrong Artist","artists":[{"id":"0000000000000000000000","name":"Pharrell Williams"}]}]}}`
const artistAlbumsJSON = `{"items":[{"id":"1111111111111111111111","name":"G I R L","album_type":"album","release_date":"2014-03-03","images":[{"url":"https://i.scdn.co/image/girl"}],"external_urls":{"spotify":"https://attacker.test"}}]}`

func artistHandlerForServer(server *httptest.Server) *ArtistHandler {
	return NewArtistHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: spot.New(server.Client(), spot.WithBaseURL(server.URL+"/"))})
}

func artistRequest(h http.Handler, query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/artist?"+query, nil))
	return w
}

func TestArtistRejectsInvalidRequestsBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	h := artistHandlerForServer(server)
	for _, query := range []string{
		"", "spotifyId=short", "spotifyId=" + artistID + "a", "spotifyId=" + strings.Repeat("-", 22),
		"spotifyId=" + artistID + "&spotifyId=" + artistID,
		"spotifyId=" + artistID + "&offset=-1", "spotifyId=" + artistID + "&offset=991",
		"spotifyId=" + artistID + "&offset=1.5", "spotifyId=" + artistID + "&offset=",
		"spotifyId=" + artistID + "&offset=+1", "spotifyId=" + artistID + "&offset=0&offset=10",
	} {
		w := artistRequest(h, query)
		if w.Code != 400 || !json.Valid(w.Body.Bytes()) {
			t.Errorf("query %q: %d %s", query, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/artist?spotifyId="+artistID, nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET" || !json.Valid(w.Body.Bytes()) {
		t.Errorf("method response = %d %s", w.Code, w.Body.String())
	}
	if calls.Load() != 0 {
		t.Errorf("invalid requests reached Spotify: %d", calls.Load())
	}
}

func TestArtistUsesExactArtistTracksAndCanonicalCatalogLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artists/" + artistID:
			io.WriteString(w, artistJSON)
		case "/search":
			q := r.URL.Query()
			if q.Get("q") != `artist:"Pharrell Williams"` || q.Get("limit") != "10" || q.Get("offset") != "0" || q.Get("type") != "track" {
				t.Errorf("wrong track search: %s", r.URL.RawQuery)
			}
			io.WriteString(w, artistTracksJSON)
		case "/artists/" + artistID + "/albums":
			if r.URL.Query().Get("include_groups") != "album,single" || r.URL.Query().Get("limit") != "10" {
				t.Errorf("wrong albums query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, artistAlbumsJSON)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	w := artistRequest(artistHandlerForServer(server), "spotifyId="+artistID)
	var body struct {
		Artist struct {
			ID, Name, URL string
			Image         *string
			Genres        []string
		}
		Tracks []SearchTrack
		Albums []struct {
			ID, Name, URL string
			Image         *string
			ReleaseDate   string `json:"release_date"`
			AlbumType     string `json:"album_type"`
		}
		NextOffset  *int `json:"next_offset"`
		Unavailable []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
	if body.Artist.ID != artistID || body.Artist.Name != "Pharrell Williams" || body.Artist.URL != "https://open.spotify.com/artist/2RdwBSPQiwcmiDo9kixcl8" || body.Artist.Image == nil || len(body.Artist.Genres) != 1 {
		t.Errorf("wrong artist: %s", w.Body.String())
	}
	if len(body.Tracks) != 1 || body.Tracks[0].ID != "happy" || body.Tracks[0].Thumb == nil {
		t.Errorf("wrong artist tracks: %+v", body.Tracks)
	}
	if len(body.Albums) != 1 || body.Albums[0].URL != "https://open.spotify.com/album/1111111111111111111111" || body.Albums[0].Image == nil || body.Albums[0].ReleaseDate != "2014-03-03" || body.Albums[0].AlbumType != "album" {
		t.Errorf("wrong albums: %+v", body.Albums)
	}
	if body.NextOffset == nil || *body.NextOffset != 10 || body.Unavailable == nil || len(body.Unavailable) != 0 {
		t.Errorf("wrong pagination or availability: %s", w.Body.String())
	}
}

func TestArtistKeepsPagingAfterFilteredEmptyPageAndStopsAt990(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artists/" + artistID:
			io.WriteString(w, artistJSON)
		case "/search":
			io.WriteString(w, `{"tracks":{"next":"https://api.spotify.com/v1/search?offset=1000","total":2000,"items":[{"id":"wrong","artists":[{"id":"0000000000000000000000"}]}]}}`)
		default:
			io.WriteString(w, `{"items":[]}`)
		}
	}))
	defer server.Close()
	h := artistHandlerForServer(server)
	for _, tc := range []struct {
		offset string
		next   string
	}{{"20", "30"}, {"990", "null"}} {
		w := artistRequest(h, "spotifyId="+artistID+"&offset="+tc.offset)
		var body map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || string(body["tracks"]) != "[]" || string(body["next_offset"]) != tc.next || string(body["albums"]) != "[]" {
			t.Fatalf("response = %d %s", w.Code, w.Body.String())
		}
	}
}

func TestArtistRequiresValidIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"not found", 404, `{"error":{"status":404,"message":"private details"}}`, 404},
		{"upstream failure", 500, `{"error":{"status":500,"message":"private details"}}`, 502},
		{"wrong identity", 200, `{"id":"0000000000000000000000","name":"Wrong"}`, 502},
		{"missing name", 200, `{"id":"2RdwBSPQiwcmiDo9kixcl8"}`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/artists/"+artistID {
					t.Error("optional data requested without artist identity")
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			w := artistRequest(artistHandlerForServer(server), "spotifyId="+artistID)
			if w.Code != tc.want || !json.Valid(w.Body.Bytes()) || strings.Contains(w.Body.String(), "private") {
				t.Errorf("response = %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestArtistLoadsSectionsIndependentlyAndDoesNotCachePartialData(t *testing.T) {
	for _, failed := range []string{"tracks", "albums"} {
		t.Run(failed, func(t *testing.T) {
			var identityCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/artists/" + artistID:
					identityCalls.Add(1)
					io.WriteString(w, artistJSON)
				case "/search":
					if failed == "tracks" {
						w.WriteHeader(500)
						io.WriteString(w, `{"error":{"status":500,"message":"private"}}`)
						return
					}
					io.WriteString(w, artistTracksJSON)
				default:
					if failed == "albums" {
						w.WriteHeader(500)
						io.WriteString(w, `{"error":{"status":500,"message":"private"}}`)
						return
					}
					io.WriteString(w, artistAlbumsJSON)
				}
			}))
			defer server.Close()
			h := artistHandlerForServer(server)
			for i := 0; i < 2; i++ {
				w := artistRequest(h, "spotifyId="+artistID)
				var body struct {
					Tracks      []SearchTrack
					Albums      []json.RawMessage
					Unavailable []string
					NextOffset  *int `json:"next_offset"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
					t.Fatalf("response = %d %s", w.Code, w.Body.String())
				}
				if len(body.Unavailable) != 1 || body.Unavailable[0] != failed || body.Tracks == nil || body.Albums == nil {
					t.Errorf("wrong partial response: %s", w.Body.String())
				}
				if failed == "tracks" && (len(body.Albums) != 1 || body.NextOffset != nil) {
					t.Errorf("album data lost: %s", w.Body.String())
				}
				if failed == "albums" && len(body.Tracks) != 1 {
					t.Errorf("track data lost: %s", w.Body.String())
				}
			}
			if identityCalls.Load() != 2 {
				t.Errorf("partial data was cached")
			}
		})
	}
}

func TestArtistCachesCompleteDataByOffset(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/artists/" + artistID:
			io.WriteString(w, artistJSON)
		case "/search":
			io.WriteString(w, `{"tracks":{"items":[]}}`)
		default:
			io.WriteString(w, `{"items":[]}`)
		}
	}))
	defer server.Close()
	h := artistHandlerForServer(server)
	first := artistRequest(h, "spotifyId="+artistID)
	second := artistRequest(h, "spotifyId="+artistID)
	if first.Code != 200 || second.Body.String() != first.Body.String() || calls.Load() != 3 {
		t.Fatalf("complete response was not cached; calls=%d", calls.Load())
	}
	artistRequest(h, "spotifyId="+artistID+"&offset=10")
	if calls.Load() != 6 {
		t.Errorf("offset cache collision; calls=%d", calls.Load())
	}
}

func TestArtistSharesSixSecondDeadlineAndPropagatesCancellation(t *testing.T) {
	var mu sync.Mutex
	var deadline time.Time
	c := spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		d, ok := r.Context().Deadline()
		mu.Lock()
		if !ok || time.Until(d) > 6*time.Second {
			t.Error("artist load exceeds six seconds")
		}
		if deadline.IsZero() {
			deadline = d
		} else if !d.Equal(deadline) {
			t.Error("artist sections do not share one deadline")
		}
		mu.Unlock()
		w := httptest.NewRecorder()
		switch r.URL.Path {
		case "/v1/artists/" + artistID:
			io.WriteString(w, artistJSON)
		case "/v1/search":
			io.WriteString(w, `{"tracks":{"items":[]}}`)
		default:
			io.WriteString(w, `{"items":[]}`)
		}
		return w.Result(), nil
	})})
	h := NewArtistHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: c})
	if w := artistRequest(h, "spotifyId="+artistID); w.Code != 200 {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}

	c = spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Context().Err() != context.Canceled {
			t.Error("parent cancellation lost")
		}
		return nil, r.Context().Err()
	})})
	h = NewArtistHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: c})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/artist?spotifyId="+artistID, nil).WithContext(ctx))
	if w.Body.Len() != 0 {
		t.Errorf("wrote data after cancellation: %s", w.Body.String())
	}
}

func TestArtistBoundsActiveLoadsAndCancelsWaitingRequests(t *testing.T) {
	started := make(chan struct{}, 8)
	var calls atomic.Int32
	c := spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})})
	h := NewArtistHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: c})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	defer wg.Wait()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/artist?spotifyId="+artistID, nil).WithContext(ctx))
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("active loads did not start")
		}
	}
	waiting, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/artist?spotifyId="+artistID, nil).WithContext(waiting))
	if w.Code != 504 || !json.Valid(w.Body.Bytes()) || calls.Load() != 8 {
		t.Errorf("waiting request was not bounded: %d %s, calls=%d", w.Code, w.Body.String(), calls.Load())
	}
	cancel()
}

func TestArtistCacheExpiresAndBoundsStoredResponses(t *testing.T) {
	h := NewArtistHandler(zap.NewNop().Sugar(), nil)
	now := time.Now()
	key := artistCacheKey{id: artistID}
	h.remember(key, []byte(`{"artist":{}}`), now)
	if _, ok := h.cached(key, now.Add(5*time.Minute-time.Nanosecond)); !ok {
		t.Error("fresh response missing")
	}
	if _, ok := h.cached(key, now.Add(5*time.Minute)); ok {
		t.Error("five-minute response was not expired")
	}
	for i := 0; i < 150; i++ {
		h.remember(artistCacheKey{id: artistID, offset: i}, []byte(`{}`), now.Add(time.Duration(i)*time.Millisecond))
	}
	if len(h.cache) > 128 {
		t.Errorf("cache grew without a bound: %d", len(h.cache))
	}
	if _, ok := h.cached(key, now); ok {
		t.Error("oldest entry was not evicted")
	}
	oversized := artistCacheKey{id: "oversized"}
	h.remember(oversized, []byte(strings.Repeat(" ", 128*1024+1)), now)
	if _, ok := h.cached(oversized, now); ok {
		t.Error("oversized response was cached")
	}
	h.remember(artistCacheKey{id: "new"}, []byte(`{}`), now.Add(6*time.Minute))
	if len(h.cache) != 1 {
		t.Errorf("expired entries were not removed: %d", len(h.cache))
	}
}
