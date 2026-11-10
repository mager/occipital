package spotify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCatalogAlbumBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"catalog", `{"id":"album","label":"A label","total_tracks":12,"copyrights":[{"text":"2026 Artist","type":"P"}]}`, 200, true},
		{"wrong identity", `{"id":"other"}`, 200, false},
		{"unavailable", `{}`, 503, false},
		{"malformed", `no`, 200, false},
		{"oversize", strings.Repeat("x", 1024*1024+1), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &SpotifyClient{HTTP: &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.spotify.com/v1/albums/album" {
					t.Fatal(r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}, Request: r}, nil
			})}}
			album, err := c.GetCatalogAlbum(context.Background(), "album")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (album.Label != "A label" || album.TotalTracks != 12 || len(album.Copyrights) != 1) {
				t.Fatalf("lost fields: %+v", album)
			}
		})
	}
}
func TestCatalogHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &SpotifyClient{HTTP: &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}}
	if _, err := c.GetCatalogAlbum(ctx, "album"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
