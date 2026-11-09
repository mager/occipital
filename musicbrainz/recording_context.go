package musicbrainz

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"time"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"golang.org/x/time/rate"
)

// Share one request slot per second across all v2 track lookups in this process.
// Waiting uses the request context, so queued metadata does not extend its deadline.
var recordingRequestLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

// RecordingMatch retains evidence needed to distinguish recordings with the same title.
type RecordingMatch struct {
	mb.Recording
	Length         int    `json:"length"`
	Disambiguation string `json:"disambiguation"`
}

type RecordingSearch struct {
	Count      int              `json:"count"`
	Recordings []RecordingMatch `json:"recordings"`
}

// getContext is used by the v2 track route. The older shared client has no context API.
func (c *MusicbrainzClient) getContext(ctx context.Context, path string, query url.Values, out any) error {
	query.Set("fmt", "json")
	u := &url.URL{Scheme: "https", Host: "musicbrainz.org", Path: "/ws/2/" + path, RawQuery: query.Encode()}
	for attempt := 0; attempt < 2; attempt++ {
		if err := recordingRequestLimiter.Wait(ctx); err != nil {
			return err
		}
		err := c.getContextOnce(ctx, u, out)
		if err == nil || attempt == 1 || !retryableRecordingError(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var statusErr *recordingStatusError
		if errors.As(err, &statusErr) && statusErr.retryAfter > 0 {
			// Honor Retry-After only when another attempt fits the existing deadline.
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= statusErr.retryAfter {
				return err
			}
			timer := time.NewTimer(statusErr.retryAfter)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}

func (c *MusicbrainzClient) getContextOnce(ctx context.Context, u *url.URL, out any) (err error) {
	// Only retain a fixed stage label. Do not record headers, addresses, or credentials.
	var stageMu sync.Mutex
	stage := "request_start"
	setStage := func(value string) { stageMu.Lock(); stage = value; stageMu.Unlock() }
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { setStage("dns") },
		DNSDone: func(info httptrace.DNSDoneInfo) {
			if info.Err == nil {
				setStage("dns_complete")
			}
		},
		ConnectStart: func(string, string) { setStage("connect") },
		ConnectDone: func(_ string, _ string, err error) {
			if err == nil {
				setStage("connect_complete")
			}
		},
		TLSHandshakeStart: func() { setStage("tls") },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				setStage("tls_complete")
			}
		},
		GotConn: func(httptrace.GotConnInfo) { setStage("connected") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				setStage("request_written")
			} else {
				setStage("request_write")
			}
		},
		GotFirstResponseByte: func() { setStage("first_response_byte") },
	}
	ctx = httptrace.WithClientTrace(ctx, trace)
	defer func() {
		if err != nil {
			stageMu.Lock()
			lastStage := stage
			stageMu.Unlock()
			err = fmt.Errorf("MusicBrainz request failed at %s: %w", lastStage, err)
		}
	}()
	req := c.Client.GetRequest(u).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		delay := time.Duration(0)
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		} else if date, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil {
			delay = time.Until(date)
		}
		return &recordingStatusError{status: resp.StatusCode, retryAfter: delay}
	}
	// Decode the complete document before changing the output. A truncated first
	// attempt must not leave fields behind when a retry returns a different body.
	var data json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

type recordingStatusError struct {
	status     int
	retryAfter time.Duration
}

func (e *recordingStatusError) Error() string { return fmt.Sprintf("MusicBrainz status %d", e.status) }

func retryableRecordingError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusErr *recordingStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status == 429 || statusErr.status == 502 || statusErr.status == 503 || statusErr.status == 504
	}
	var networkErr net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &networkErr)
}

func (c *MusicbrainzClient) SearchRecordingsContext(ctx context.Context, query string) (RecordingSearch, error) {
	var result RecordingSearch
	err := c.getContext(ctx, "recording", url.Values{"query": {query}, "limit": {"25"}}, &result)
	return result, err
}

// RecordingDetails keeps nested work credits that the older shared SDK drops.
// Requesting work-level-rels removes a separate network call for songwriting.
type RecordingDetails struct {
	mb.Recording
	Length int                `json:"length"`
	Video  bool               `json:"video"`
	Works  map[string]mb.Work `json:"-"`
}

func (r *RecordingDetails) UnmarshalJSON(data []byte) error {
	type plain RecordingDetails
	var result plain
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	var nested struct {
		Relations []struct {
			TargetType string   `json:"target-type"`
			Work       *mb.Work `json:"work"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(data, &nested); err != nil {
		return err
	}
	result.Works = make(map[string]mb.Work)
	for _, relation := range nested.Relations {
		if relation.TargetType == "work" && relation.Work != nil && relation.Work.ID != "" && relation.Work.Relations != nil {
			result.Works[relation.Work.ID] = *relation.Work
		}
	}
	*r = RecordingDetails(result)
	return nil
}

func (c *MusicbrainzClient) GetRecordingContext(ctx context.Context, id string) (RecordingDetails, error) {
	var result RecordingDetails
	err := c.getContext(ctx, "recording/"+url.PathEscape(id), url.Values{"inc": {"artist-credits+artist-rels+genres+isrcs+releases+work-rels+work-level-rels+url-rels"}}, &result)
	return result, err
}

// LookupRecordingsByURL uses MusicBrainz's explicit link to this Spotify track,
// not a title search. A missing URL is different from a failed lookup.
func (c *MusicbrainzClient) LookupRecordingsByURL(ctx context.Context, resource string) ([]string, bool, error) {
	var result struct {
		Resource  string `json:"resource"`
		Relations []struct {
			TargetType string `json:"target-type"`
			Type       string `json:"type"`
			Direction  string `json:"direction"`
			Ended      bool   `json:"ended"`
			Recording  struct {
				ID    string `json:"id"`
				Video bool   `json:"video"`
			} `json:"recording"`
		} `json:"relations"`
	}
	err := c.getContext(ctx, "url", url.Values{"resource": {resource}, "inc": {"recording-rels"}}, &result)
	if err != nil {
		var status *recordingStatusError
		if errors.As(err, &status) && status.status == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if result.Resource != resource {
		return nil, true, fmt.Errorf("MusicBrainz returned an unexpected URL")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, relation := range result.Relations {
		id := relation.Recording.ID
		if relation.TargetType == "recording" && relation.Direction == "backward" &&
			(relation.Type == "free streaming" || relation.Type == "streaming") && !relation.Ended && !relation.Recording.Video && id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, true, nil
}

func (c *MusicbrainzClient) GetWorkContext(ctx context.Context, id string) (mb.Work, error) {
	var result mb.Work
	err := c.getContext(ctx, "work/"+url.PathEscape(id), url.Values{"inc": {"artist-rels+url-rels"}}, &result)
	return result, err
}
