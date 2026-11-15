package creator

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mager/occipital/musicbrainz"
	"go.uber.org/zap"
)

const personID = "149f91ef-1287-46da-9a8e-87fee02f1471"
const groupID = "253bfe58-d68a-4f96-9b7a-70d977cbf66d"
const songID = "b5e1e12f-8832-4ba4-b772-de15e2db2009"
const creatorFixture = `{"id":"149f91ef-1287-46da-9a8e-87fee02f1471","name":"Pharrell Williams","type":"Person","relations":[
 {"type":"producer","target-type":"recording","recording":{"id":"b5e1e12f-8832-4ba4-b772-de15e2db2009","title":"Alright","first-release-date":"2015-03-15","disambiguation":"explicit","artist-credit":[{"name":"Kendrick Lamar","artist":{"id":"381086ea-f511-4aba-bdf9-71c753dc5077","name":"Kendrick Lamar"}}]}},
 {"type":"producer","target-type":"recording","recording":{"id":"b5e1e12f-8832-4ba4-b772-de15e2db2009","title":"Alright"}},
 {"type":"vocal","attributes":["background vocals"],"target-type":"recording","recording":{"id":"b5e1e12f-8832-4ba4-b772-de15e2db2009","title":"Alright"}},
 {"type":"vocal","target-type":"recording","recording":{"id":"833f00e1-781f-4edd-90e4-e52712618862","title":"Get Lucky"}},
 {"type":"producer","attributes":["executive"],"target-type":"release","release":{"id":"10000000-0000-4000-8000-000000000001","title":"An album","date":"2006"}},
 {"type":"writer","target-type":"work","work":{"id":"10000000-0000-4000-8000-000000000002","title":"A composition"}},
 {"type":"member of band","direction":"forward","begin":"1990","end":"2024","target-type":"artist","artist":{"id":"253bfe58-d68a-4f96-9b7a-70d977cbf66d","name":"The Neptunes"}},
 {"type":"producer","target-type":"recording","recording":{"id":"bad","title":"Invalid"}},
 {"type":"producer","target-type":"recording","recording":{"id":"10000000-0000-4000-8000-000000000003","title":""}}
 ]}`

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func installFixture(t *testing.T, body string, status int) *int {
	t.Helper()
	previous := http.DefaultTransport
	calls := new(int)
	http.DefaultTransport = roundTrip(func(r *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	return calls
}

func TestCatalogPreservesCreditIdentityAndEntityBoundaries(t *testing.T) {
	calls := installFixture(t, creatorFixture, 200)
	h := NewGetCreatorHandler(zap.NewNop().Sugar(), musicbrainz.ProvideMusicbrainz(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/creator?mbid="+personID, nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var result struct {
		Creator struct {
			Catalog []struct {
				ID, Kind, Title, Date, Disambiguation string
				Roles                                 []string
				Artists                               []struct{ ID, Name string }
			}
			Connections []struct{ ID, Name, Relation, Begin, End string }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	c := result.Creator
	if len(c.Catalog) != 4 {
		t.Fatalf("want 4 distinct valid entities, got %+v", c.Catalog)
	}
	for _, item := range c.Catalog {
		if item.ID == songID && (len(item.Roles) != 2 || item.Date != "2015-03-15" || len(item.Artists) != 1 || item.Disambiguation != "explicit") {
			t.Fatalf("lost recording details: %+v", item)
		}
		if item.Title == "Get Lucky" && strings.Contains(strings.Join(item.Roles, " "), "producer") {
			t.Fatal("vocal credit became production")
		}
		if item.Kind == "release" && strings.Join(item.Roles, ",") != "executive producer" {
			t.Fatalf("lost executive scope: %+v", item)
		}
	}
	if len(c.Connections) != 1 || c.Connections[0].Relation != "Member of" || c.Connections[0].End != "2024" {
		t.Fatalf("bad group connection: %+v", c.Connections)
	}
	again := httptest.NewRecorder()
	h.ServeHTTP(again, httptest.NewRequest("GET", "/creator?mbid="+personID, nil))
	if *calls != 1 {
		t.Fatalf("successful public metadata was not cached: %d calls", *calls)
	}
}

func TestCreatorRejectsInvalidInputBeforeProvider(t *testing.T) {
	calls := installFixture(t, creatorFixture, 200)
	h := NewGetCreatorHandler(zap.NewNop().Sugar(), musicbrainz.ProvideMusicbrainz(), nil)
	for _, query := range []string{"", "bad", personID + "&mbid=" + groupID} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator?mbid="+query, nil))
		if w.Code != 400 {
			t.Errorf("query %q: got %d", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/creator?mbid="+personID, nil))
	if w.Code != 405 {
		t.Errorf("POST status %d", w.Code)
	}
	if *calls != 0 {
		t.Fatalf("invalid input made %d provider requests", *calls)
	}
}

func TestCreatorProviderFailureIsNotAnEmptyCatalog(t *testing.T) {
	installFixture(t, `{"error":"not found"}`, 404)
	h := NewGetCreatorHandler(zap.NewNop().Sugar(), musicbrainz.ProvideMusicbrainz(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/creator?mbid="+personID, nil))
	if w.Code != 404 {
		t.Fatalf("got %d, want 404", w.Code)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("error must be JSON")
	}
	var body map[string]any
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"] == nil {
		t.Fatal("missing JSON error")
	}
}

func TestConcurrentCreatorReadsShareOneProviderRequest(t *testing.T) {
	previous := http.DefaultTransport
	var mu sync.Mutex
	calls := 0
	started := make(chan struct{})
	release := make(chan struct{})
	http.DefaultTransport = roundTrip(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			close(started)
		}
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(creatorFixture)), Header: http.Header{}, Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	h := NewGetCreatorHandler(zap.NewNop().Sugar(), musicbrainz.ProvideMusicbrainz(), nil)
	done := make(chan int, 2)
	run := func() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/creator?mbid="+personID, nil))
		done <- w.Code
	}
	go run()
	<-started
	go run()
	time.Sleep(25 * time.Millisecond)
	close(release)
	for i := 0; i < 2; i++ {
		if status := <-done; status != 200 {
			t.Fatalf("status %d", status)
		}
	}
	if calls != 1 {
		t.Fatalf("simultaneous creator reads made %d requests", calls)
	}
}
