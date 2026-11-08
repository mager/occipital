package track

import (
	"encoding/json"
	"reflect"
	"testing"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/occipital"
)

func TestProductionCreditsKeepTechnicalRoles(t *testing.T) {
	var rec mb.Recording
	if err := json.Unmarshal([]byte(`{"relations":[
		{"type":"engineer","artist":{"id":"bryce","name":"Bryce Bordone"}},
		{"type":"engineer","artist":{"id":"jack","name":"Jack Manning"}},
		{"type":"engineer","artist":{"id":"bryce","name":"Bryce Bordone"}},
		{"type":"programming","artist":{"id":"antonoff","name":"Jack Antonoff"}},
		{"type":"mastering","artist":{"id":"randy","name":"Randy Merrill"}},
		{"type":"engineer"},
		{"type":"phonographic copyright","artist":{"id":"taylor","name":"Taylor Swift"}}
	]}`), &rec); err != nil {
		t.Fatal(err)
	}
	want := []*occipital.TrackProductionCredit{
		{Credit: "engineer", Artists: []occipital.CreditArtist{{ID: "bryce", Name: "Bryce Bordone"}, {ID: "jack", Name: "Jack Manning"}}},
		{Credit: "mastering", Artists: []occipital.CreditArtist{{ID: "randy", Name: "Randy Merrill"}}},
		{Credit: "programming", Artists: []occipital.CreditArtist{{ID: "antonoff", Name: "Jack Antonoff"}}},
	}
	if got := getProductionCreditsForRecording(rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("missing, duplicate, or unordered technical credits: %+v", got)
	}
}
