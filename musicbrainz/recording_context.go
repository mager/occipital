package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/time/rate"
	"io"
	"net/http"
	"net/url"
	"time"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
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
	if err := recordingRequestLimiter.Wait(ctx); err != nil {
		return err
	}
	query.Set("fmt", "json")
	u := &url.URL{Scheme: "https", Host: "musicbrainz.org", Path: "/ws/2/" + path, RawQuery: query.Encode()}
	req := c.Client.GetRequest(u).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MusicBrainz status %d", resp.StatusCode)
	}
	// Bound upstream responses. Truncated JSON fails decoding instead of being cached.
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}

func (c *MusicbrainzClient) SearchRecordingsContext(ctx context.Context, query string) (RecordingSearch, error) {
	var result RecordingSearch
	err := c.getContext(ctx, "recording", url.Values{"query": {query}, "limit": {"25"}}, &result)
	return result, err
}

func (c *MusicbrainzClient) GetRecordingContext(ctx context.Context, id string) (mb.Recording, error) {
	var result mb.Recording
	err := c.getContext(ctx, "recording/"+url.PathEscape(id), url.Values{"inc": {"artist-credits+artist-rels+genres+isrcs+releases+work-rels+url-rels"}}, &result)
	return result, err
}

func (c *MusicbrainzClient) GetWorkContext(ctx context.Context, id string) (mb.Work, error) {
	var result mb.Work
	err := c.getContext(ctx, "work/"+url.PathEscape(id), url.Values{"inc": {"artist-rels+url-rels"}}, &result)
	return result, err
}
