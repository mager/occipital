package spotify

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mager/occipital/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSpotifyTokenRequestHasTimeout(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Error("Spotify token request has no finite timeout")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`)), Request: r}, nil
	})
	ProvideSpotify(config.Config{SpotifyID: "test", SpotifySecret: "test"})
}
