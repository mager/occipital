package creator

import (
	"go.uber.org/zap"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryCachesAndBoundsProviderRequests(t *testing.T) {
	calls := installFixture(t, `{"056e4f3e-d505-4dad-8ec1-d04f521cbb56":[]}`, 200)
	h := NewGetCreatorDiscoveryHandler(zap.NewNop().Sugar())
	for _, q := range []string{"", "mbid=bad", "mbid=" + personID + "&recordings=" + songID, "recordings=bad"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator/discovery?"+q, nil))
		if w.Code != 400 {
			t.Fatalf("query %s status %d", q, w.Code)
		}
	}
	if *calls != 0 {
		t.Fatal("invalid request reached provider")
	}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator/discovery?mbid="+personID, nil))
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
	}
	if *calls != 1 {
		t.Fatalf("cache made %d calls", *calls)
	}
}

func TestDiscoveryRejectsUnknownProviderPayloadBeforeCaching(t *testing.T) {
	calls := installFixture(t, `{"message":"Service unavailable"}`, 200)
	h := NewGetCreatorDiscoveryHandler(zap.NewNop().Sugar())
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator/discovery?mbid="+personID, nil))
		if w.Code != 503 {
			t.Fatalf("got %d", w.Code)
		}
	}
	if *calls != 2 {
		t.Fatal("malformed response was cached")
	}
}

func TestDiscoveryFailureDoesNotPoisonCache(t *testing.T) {
	calls := installFixture(t, `{"error":"upstream detail"}`, 200)
	h := NewGetCreatorDiscoveryHandler(zap.NewNop().Sugar())
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator/discovery?recordings="+songID, nil))
		if w.Code != 503 {
			t.Fatalf("got %d", w.Code)
		}
	}
	if *calls != 2 {
		t.Fatal("error was cached")
	}
}
