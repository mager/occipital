package musicbrainz

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
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

func TestRecordingRequestRetriesTransientFailuresOnce(t *testing.T) {
	for _, failure := range []string{"EOF", "503", "429", "truncated body"} {
		t.Run(failure, func(t *testing.T) {
			installRecordingTransport(t, time.Millisecond)
			attempts := 0
			http.DefaultTransport = contextRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				status, body := 200, `{"count":1,"recordings":[{"id":"recovered"}]}`
				if attempts == 1 {
					switch failure {
					case "EOF":
						return nil, io.EOF
					case "503":
						status = 503
					case "429":
						status = 429
					case "truncated body":
						body = `{"count":`
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
			})
			result, err := ProvideMusicbrainz().SearchRecordingsContext(context.Background(), "test")
			if err != nil || result.Count != 1 || len(result.Recordings) != 1 || result.Recordings[0].ID != "recovered" || attempts != 2 {
				t.Fatalf("retry did not recover: %+v, %v, attempts=%d", result, err, attempts)
			}
		})
	}
}

func TestRecordingRequestDoesNotRetryForeverOrRetryPermanentErrors(t *testing.T) {
	for _, status := range []int{503, 400} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			installRecordingTransport(t, time.Millisecond)
			attempts := 0
			http.DefaultTransport = contextRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}, Request: r}, nil
			})
			_, err := ProvideMusicbrainz().SearchRecordingsContext(context.Background(), "test")
			want := 1
			if status == 503 {
				want = 2
			}
			if err == nil || attempts != want {
				t.Fatalf("got %v after %d requests, want error after %d", err, attempts, want)
			}
		})
	}
}

func TestRecordingRetryRespectsCancellationAndRetryAfter(t *testing.T) {
	for _, mode := range []string{"cancelled", "retry-after exceeds deadline"} {
		t.Run(mode, func(t *testing.T) {
			installRecordingTransport(t, time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			attempts := 0
			http.DefaultTransport = contextRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				if mode == "cancelled" {
					cancel()
					return nil, io.EOF
				}
				return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Retry-After": []string{"60"}}, Request: r}, nil
			})
			_, err := ProvideMusicbrainz().SearchRecordingsContext(ctx, "test")
			if err == nil || attempts != 1 {
				t.Fatalf("request must not retry: %v, attempts=%d", err, attempts)
			}
		})
	}
}

func TestRecordingRequestErrorIncludesStageAndPreservesCause(t *testing.T) {
	for _, stage := range []string{"dns", "connect", "tls", "request_written", "first_response_byte"} {
		t.Run(stage, func(t *testing.T) {
			installRecordingTransport(t, time.Millisecond)
			http.DefaultTransport = contextRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				trace := httptrace.ContextClientTrace(r.Context())
				if trace == nil {
					return nil, io.EOF
				}
				switch stage {
				case "dns":
					trace.DNSStart(httptrace.DNSStartInfo{})
				case "connect":
					trace.ConnectStart("tcp", "unused")
				case "tls":
					trace.TLSHandshakeStart()
				case "request_written":
					trace.WroteRequest(httptrace.WroteRequestInfo{})
				case "first_response_byte":
					trace.GotFirstResponseByte()
				}
				return nil, io.EOF
			})
			u, _ := url.Parse("https://musicbrainz.org/ws/2/recording")
			var result RecordingSearch
			err := ProvideMusicbrainz().getContextOnce(context.Background(), u, &result)
			if !errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "at "+stage+":") {
				t.Fatalf("missing stage or original cause: %v", err)
			}
		})
	}
}
