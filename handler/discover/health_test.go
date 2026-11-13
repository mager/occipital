package discover

import (
	"context"
	fs "github.com/mager/occipital/firestore"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceHealthDoesNotHideOldOrFailedSources(t *testing.T) {
	now := time.Date(2026, 11, 13, 14, 0, 0, 0, time.UTC)
	tracks := []fs.Track{{SpotifyID: "aaaaaaaaaaaaaaaaaaaaaa", Thumb: "art"}, {Thumb: "broken"}}
	current := sourceHealth("billboard", "2026-11-13", tracks, now, sourceAttempt{})
	old := sourceHealth("apple_music_charts", "2026-11-10", tracks, now, sourceAttempt{})
	failed := sourceHealth("hnhh", "2026-11-13", tracks, now, sourceAttempt{Outcome: "failed", CheckedAt: now})
	missing := sourceHealth("pitchfork_bnm", "", nil, now, sourceAttempt{})
	if current.Status != "current" || current.Matched != 1 || current.Total != 2 {
		t.Fatalf("current=%+v", current)
	}
	if old.Status != "delayed" || old.Updated != "2026-11-10" || failed.Status != "delayed" || missing.Status != "unavailable" {
		t.Fatalf("health: %+v %+v %+v", old, failed, missing)
	}
}

func TestDiscoveryCacheCoalescesConcurrentReads(t *testing.T) {
	var cache responseCache
	var calls atomic.Int32
	start := make(chan struct{})
	entered := make(chan struct{})
	load := func(context.Context) (*DiscoverResponse, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-start
		return &DiscoverResponse{Updated: "2026-11-13"}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := cache.get(context.Background(), load)
			if err != nil || response.Updated != "2026-11-13" {
				t.Error("cached response lost")
			}
		}()
	}
	<-entered
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate Firestore reads: %d", calls.Load())
	}
	cache.expires = time.Now().Add(-time.Second)
	_, _ = cache.get(context.Background(), load)
	if calls.Load() != 2 {
		t.Fatal("expired source status was never refreshed")
	}
}

func TestDiscoveryCacheWaitCanBeCanceled(t *testing.T) {
	cache := responseCache{loading: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cache.get(ctx, func(context.Context) (*DiscoverResponse, error) { t.Fatal("unexpected load"); return nil, nil })
	if err != context.Canceled {
		t.Fatalf("canceled caller waited: %v", err)
	}
}

func TestDiscoveryCacheDoesNotCacheFailedLoads(t *testing.T) {
	var cache responseCache
	calls := 0
	load := func(context.Context) (*DiscoverResponse, error) {
		calls++
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		return &DiscoverResponse{Updated: "2026-11-13"}, nil
	}
	if _, err := cache.get(context.Background(), load); err == nil {
		t.Fatal("failed load returned success")
	}
	response, err := cache.get(context.Background(), load)
	if err != nil || response.Updated != "2026-11-13" || calls != 2 {
		t.Fatal("failed load blocked recovery")
	}
}
