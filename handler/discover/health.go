package discover

import (
	"context"
	fs "github.com/mager/occipital/firestore"
	"sync"
	"time"
)

type SourceHealth struct {
	Source      string     `json:"source"`
	Updated     string     `json:"updated"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Matched     int        `json:"matched"`
	LastAttempt *time.Time `json:"last_attempt,omitempty"`
}
type sourceAttempt struct {
	Outcome   string    `firestore:"outcome"`
	CheckedAt time.Time `firestore:"checkedAt"`
}

func usableTrack(track fs.Track) bool {
	if track.Thumb == "" || len(track.SpotifyID) != 22 {
		return false
	}
	for _, c := range track.SpotifyID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func sourceHealth(source, date string, tracks []fs.Track, now time.Time, attempt sourceAttempt) SourceHealth {
	result := SourceHealth{Source: source, Updated: date, Status: "current", Total: len(tracks)}
	for _, track := range tracks {
		if usableTrack(track) {
			result.Matched++
		}
	}
	if date != now.UTC().Format("2006-01-02") || attempt.Outcome == "failed" {
		result.Status = "delayed"
	}
	if result.Matched == 0 {
		result.Status = "unavailable"
	}
	if !attempt.CheckedAt.IsZero() {
		result.LastAttempt = &attempt.CheckedAt
	}
	return result
}

// Discovery is shared catalog data. Cache it briefly inside the existing
// process; member and session data never enter this cache. Concurrent readers
// share a single load, and may leave the queue when their request is canceled.
type responseCache struct {
	mu      sync.Mutex
	value   *DiscoverResponse
	expires time.Time
	loading chan struct{}
}

func (c *responseCache) get(ctx context.Context, load func(context.Context) (*DiscoverResponse, error)) (*DiscoverResponse, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.value != nil && time.Now().Before(c.expires) {
			value := c.value
			c.mu.Unlock()
			return value, nil
		}
		if c.loading != nil {
			wait := c.loading
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}
		c.loading = make(chan struct{})
		c.mu.Unlock()
		value, err := load(ctx)
		c.mu.Lock()
		if err == nil && value != nil {
			c.value = value
			c.expires = time.Now().Add(2 * time.Minute)
		}
		close(c.loading)
		c.loading = nil
		c.mu.Unlock()
		return value, err
	}
}
