package track

import (
	"encoding/json"
	"testing"

	"github.com/mager/occipital/musicbrainz"
	spot "github.com/zmb3/spotify/v2"
)

// The real Spotify and MusicBrainz records share ISRC USUG12401028,
// artists, and duration, but Spotify includes the guest in its title.
func TestRecordingMatchesVerifiedFeaturedArtist(t *testing.T) {
	for _, tc := range []struct {
		name, spotifyTitle, recordingTitle string
		length                             int
		byISRC                             bool
		disambiguation                     string
		want                               string
	}{
		{"Fortnight", "Fortnight (feat. Post Malone)", "Fortnight", 228965, true, "", "match"},
		{"short feature", "Fortnight (ft. Post Malone)", "Fortnight", 228965, true, "", "match"},
		{"full feature", "Fortnight (featuring Post Malone)", "Fortnight", 228965, true, "", "match"},
		{"source reversed", "Fortnight", "Fortnight (feat. Post Malone)", 228965, true, "", "match"},
		{"case and spacing", "FORTNIGHT (feat.  POST MALONE)", "Fortnight", 228965, true, "", "match"},
		{"wrong guest", "Fortnight (feat. Someone Else)", "Fortnight", 228965, true, "", ""},
		{"primary is not guest", "Fortnight (feat. Taylor Swift)", "Fortnight", 228965, true, "", ""},
		{"no ISRC evidence", "Fortnight (feat. Post Malone)", "Fortnight", 228965, false, "", ""},
		{"different duration", "Fortnight (feat. Post Malone)", "Fortnight", 210000, true, "", ""},
		{"unknown duration", "Fortnight (feat. Post Malone)", "Fortnight", 0, true, "", ""},
		{"live edition", "Fortnight (feat. Post Malone)", "Fortnight (live)", 228965, true, "", ""},
		{"remix edition", "Fortnight (remix) (feat. Post Malone)", "Fortnight", 228965, true, "", ""},
		{"edition disambiguation", "Fortnight (feat. Post Malone)", "Fortnight", 228965, true, "acoustic", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ft spot.FullTrack
			if err := json.Unmarshal([]byte(`{"duration_ms":228965,"artists":[{"name":"Taylor Swift"},{"name":"Post Malone"}]}`), &ft); err != nil {
				t.Fatal(err)
			}
			ft.Name = tc.spotifyTitle
			var rec musicbrainz.RecordingMatch
			if err := json.Unmarshal([]byte(`{"id":"match","artist-credit":[{"artist":{"name":"Taylor Swift"}},{"artist":{"name":"Post Malone"}}]}`), &rec); err != nil {
				t.Fatal(err)
			}
			rec.Title, rec.Length, rec.Disambiguation = tc.recordingTitle, tc.length, tc.disambiguation
			if got := selectRecording([]musicbrainz.RecordingMatch{rec}, &ft, tc.byISRC); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			other := rec
			other.ID = "other-recording"
			if got := selectRecording([]musicbrainz.RecordingMatch{rec, other}, &ft, tc.byISRC); got != "" {
				t.Fatalf("accepted ambiguous recording %q", got)
			}
			ft.Artists = ft.Artists[:1]
			if got := selectRecording([]musicbrainz.RecordingMatch{rec}, &ft, tc.byISRC); got != "" {
				t.Fatalf("accepted missing credited guest %q", got)
			}
		})
	}
}
