package musicbrainz

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

type contextRoundTripFunc func(*http.Request) (*http.Response, error)

func (f contextRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func installRecordingTransport(t *testing.T, interval time.Duration) *[]time.Time {
	t.Helper()
	oldTransport, oldLimiter := http.DefaultTransport, recordingRequestLimiter
	recordingRequestLimiter = rate.NewLimiter(rate.Every(interval), 1)
	var mu sync.Mutex
	starts := []time.Time{}
	http.DefaultTransport = contextRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"count":0,"recordings":[]}`)), Header: http.Header{}, Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport; recordingRequestLimiter = oldLimiter })
	return &starts
}

func TestRecordingRequestsSharePacingAcrossClients(t *testing.T) {
	starts := installRecordingTransport(t, 15*time.Millisecond)
	started := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := ProvideMusicbrainz().SearchRecordingsContext(context.Background(), "test"); err != nil {
			t.Fatal(err)
		}
	}
	if len(*starts) != 3 {
		t.Fatalf("got %d requests", len(*starts))
	}
	for i := 1; i < len(*starts); i++ {
		if elapsed := (*starts)[i].Sub(started); elapsed < time.Duration(i)*15*time.Millisecond {
			t.Fatalf("request %d started before its slot: %v", i, elapsed)
		}
	}
}

func TestRecordingLimiterWaitStopsOnCancellation(t *testing.T) {
	starts := installRecordingTransport(t, time.Second)
	client := ProvideMusicbrainz()
	if _, err := client.SearchRecordingsContext(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	started := time.Now()
	_, err := client.SearchRecordingsContext(ctx, "cancelled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("rate-limit wait ignored cancellation")
	}
	if len(*starts) != 1 {
		t.Fatal("cancelled wait sent a request")
	}
}
