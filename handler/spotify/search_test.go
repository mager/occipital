package spotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	client "github.com/mager/occipital/spotify"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

func searchHandlerForServer(server *httptest.Server) *SearchHandler {
	return NewSearchHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: spot.New(server.Client(), spot.WithBaseURL(server.URL+"/"))})
}

func TestSearchRejectsInvalidRequestsBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"tracks":{"items":[]}}`)
	}))
	defer server.Close()
	h := searchHandlerForServer(server)
	for _, tc := range []struct {
		name, method, body string
		status             int
	}{
		{"method", "GET", `{"query":"music"}`, 405},
		{"empty", "POST", `{"query":"  "}`, 400},
		{"malformed", "POST", `{"query":`, 400},
		{"trailing object", "POST", `{"query":"music"}{}`, 400},
		{"trailing junk", "POST", `{"query":"music"}x`, 400},
		{"long query", "POST", `{"query":"` + strings.Repeat("a", 201) + `"}`, 400},
		{"negative limit", "POST", `{"query":"music","limit":-1}`, 400},
		{"large body", "POST", `{"query":"music","extra":"` + strings.Repeat("a", 4096) + `"}`, 413},
		{"large trailing whitespace", "POST", `{"query":"music"}` + strings.Repeat(" ", 4096), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, "/spotify/search", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Errorf("status = %d, want %d", w.Code, tc.status)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
				t.Errorf("expected JSON error, got %q", w.Body.String())
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("not JSON content type")
			}
			if tc.status == 405 && w.Header().Get("Allow") != "POST" {
				t.Errorf("missing Allow: POST")
			}
			if calls.Load() != before {
				t.Error("invalid request reached Spotify")
			}
		})
	}
}

func TestSearchLimitsAndEmptyResults(t *testing.T) {
	for _, tc := range []struct{ body, limit string }{
		{`{"query":"  music  "}`, "20"},
		{`{"query":"  music  ","limit":3}`, "3"},
		{`{"query":"  music  ","limit":999}`, "50"},
	} {
		t.Run(tc.limit, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("q") != "music" || r.URL.Query().Get("limit") != tc.limit || r.URL.Query().Get("type") != "track" {
					t.Errorf("unexpected upstream query: %s", r.URL.RawQuery)
				}
				io.WriteString(w, `{}`)
			}))
			defer server.Close()
			w := httptest.NewRecorder()
			searchHandlerForServer(server).ServeHTTP(w, httptest.NewRequest("POST", "/spotify/search", strings.NewReader(tc.body)))
			if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"results":[]}` {
				t.Errorf("response = %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSearchPreservesTracksAndSortsPopularity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"tracks":{"items":[{"id":"low","name":"Low","popularity":2,"artists":[{"name":"Artist"}],"album":{"images":[{"url":"https://image.test/art.jpg","height":300,"width":300}]}},{"id":"high","name":"High","popularity":90,"artists":[],"album":{"images":[]}}]}}`)
	}))
	defer server.Close()
	w := httptest.NewRecorder()
	searchHandlerForServer(server).ServeHTTP(w, httptest.NewRequest("POST", "/spotify/search", strings.NewReader(`{"query":"music"}`)))
	var resp SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 || resp.Results[0].ID != "high" || resp.Results[1].Artist != "Artist" || resp.Results[1].Thumb == nil {
		t.Fatalf("unexpected tracks: %+v", resp.Results)
	}
}

func TestSearchUpstreamErrorDoesNotExposeDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":{"status":500,"message":"private upstream details"}}`)
	}))
	defer server.Close()
	w := httptest.NewRecorder()
	searchHandlerForServer(server).ServeHTTP(w, httptest.NewRequest("POST", "/spotify/search", strings.NewReader(`{"query":"music"}`)))
	if w.Code != 502 || strings.Contains(w.Body.String(), "private") || !json.Valid(w.Body.Bytes()) {
		t.Errorf("response = %d %s", w.Code, w.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchPropagatesCancellationAndDeadline(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "canceled"}[canceled], func(t *testing.T) {
			c := spot.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 10*time.Second {
					t.Error("upstream request has no bounded deadline")
				}
				if canceled && r.Context().Err() != context.Canceled {
					t.Error("parent cancellation was lost")
				}
				return nil, context.DeadlineExceeded
			})})
			h := NewSearchHandler(zap.NewNop().Sugar(), &client.SpotifyClient{Client: c})
			r := httptest.NewRequest("POST", "/spotify/search", strings.NewReader(`{"query":"music"}`))
			if canceled {
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if !canceled && (w.Code != 504 || !json.Valid(w.Body.Bytes())) {
				t.Errorf("timeout response = %d %s", w.Code, w.Body.String())
			}
		})
	}
}
