package track

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mager/occipital/musicbrainz"
	spot "github.com/zmb3/spotify/v2"
)

const directSpotifyURL = "https://open.spotify.com/track/1H4idkmruFoJBg1DvUv2tY"
const directRecordingID = "db6c7347-34e9-4023-acf4-a1ce6d0f409b"
const directSpotifyBody = `{"id":"1H4idkmruFoJBg1DvUv2tY","name":"Band On The Run - 2010 Remaster","duration_ms":313026,"artists":[{"name":"Paul McCartney"},{"name":"Wings"}],"external_ids":{"isrc":"GBCCS1000004"},"album":{"name":"Band On The Run"}}`
const directURLBody = `{"resource":"https://open.spotify.com/track/1H4idkmruFoJBg1DvUv2tY","relations":[{"target-type":"recording","type":"free streaming","direction":"backward","recording":{"id":"db6c7347-34e9-4023-acf4-a1ce6d0f409b","title":"Band on the Run","length":313093,"video":false}}]}`
const directRecordingBody = `{"id":"db6c7347-34e9-4023-acf4-a1ce6d0f409b","title":"Band on the Run","length":313093,"isrcs":["GBCCS1000004"],"artist-credit":[{"name":"Paul McCartney & Wings","artist":{"id":"wings","name":"Wings"}}],"relations":[{"target-type":"artist","type":"producer","artist":{"id":"paul","name":"Paul McCartney"}},{"target-type":"work","type":"performance","work":{"id":"work","title":"Band on the Run","relations":[{"target-type":"artist","type":"writer","artist":{"id":"paul","name":"Paul McCartney"}},{"target-type":"artist","type":"writer","artist":{"id":"linda","name":"Linda McCartney"}}]}}]}`

func TestDirectSpotifyLinkRestoresRemasterCreditsInTwoRequests(t *testing.T) {
	for _, tc := range []struct {
		name           string
		url, recording string
		want           bool
	}{
		{"linked remaster", directURLBody, directRecordingBody, true},
		{"ambiguous URL", strings.Replace(directURLBody, `]}`, `,{"target-type":"recording","type":"free streaming","direction":"backward","recording":{"id":"other","video":false}}]}`, 1), directRecordingBody, false},
		{"ended link", strings.Replace(directURLBody, `"direction":"backward"`, `"direction":"backward","ended":true`, 1), directRecordingBody, false},
		{"wrong resource", strings.ReplaceAll(directURLBody, directSpotifyURL, "https://open.spotify.com/track/other"), directRecordingBody, false},
		{"conflicting ISRC", directURLBody, strings.ReplaceAll(directRecordingBody, "GBCCS1000004", "wrong"), false},
		{"different edition", directURLBody, strings.ReplaceAll(directRecordingBody, `"Band on the Run"`, `"Band on the Run (live)"`), false},
		{"different duration", directURLBody, strings.ReplaceAll(directRecordingBody, "313093", "200000"), false},
		{"unknown duration", directURLBody, strings.ReplaceAll(directRecordingBody, "313093", "0"), false},
		{"wrong recording", directURLBody, strings.ReplaceAll(directRecordingBody, directRecordingID, "other"), false},
		{"video link", strings.ReplaceAll(directURLBody, `"video":false`, `"video":true`), directRecordingBody, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testTrackHandler(t, nil, 200)
			calls := 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				status, body := 403, `{}`
				if strings.Contains(r.URL.Host, "spotify") && strings.Contains(r.URL.Path, "/tracks/") {
					status, body = 200, directSpotifyBody
				}
				if r.URL.Host == "musicbrainz.org" {
					calls++
					switch r.URL.Path {
					case "/ws/2/url":
						if r.URL.Query().Get("resource") != directSpotifyURL {
							t.Error("wrong resource request")
						}
						status, body = 200, tc.url
					case "/ws/2/recording/" + directRecordingID:
						status, body = 200, tc.recording
					default:
						t.Errorf("unexpected metadata call: %s", r.URL.Path)
					}
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			http.DefaultTransport = transport
			h.spotifyClient.Client = spot.New(&http.Client{Transport: transport})
			got, _, err := h.fetchParallel(context.Background(), "1H4idkmruFoJBg1DvUv2tY")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want {
				if got.ID != directRecordingID || len(got.SongCredits) != 1 || len(got.SongCredits[0].Artists) != 2 || len(got.ProductionCredits) != 1 || calls != 2 {
					b, _ := json.Marshal(got)
					t.Fatalf("credits missing or extra requests (%d): %s", calls, b)
				}
			} else if got.ID != "" || len(got.SongCredits) > 0 || len(got.ProductionCredits) > 0 {
				t.Fatal("unsafe direct link accepted")
			}
		})
	}
}

func TestLinkedRecordingKeepsFeaturedTitlesAndRejectsRemixes(t *testing.T) {
	var ft spot.FullTrack
	json.Unmarshal([]byte(`{"name":"Fortnight (feat. Post Malone)","duration_ms":228965,"artists":[{"name":"Taylor Swift"},{"name":"Post Malone"}],"external_ids":{"isrc":"USUG12401028"}}`), &ft)
	var rec musicbrainz.RecordingDetails
	json.Unmarshal([]byte(`{"title":"Fortnight","length":228965,"isrcs":["USUG12401028"]}`), &rec)
	if !linkedRecordingMatches(rec, &ft) {
		t.Fatal("explicit link lost verified featured artist title")
	}
	ft.Name = "Fortnight (remix) (feat. Post Malone)"
	if linkedRecordingMatches(rec, &ft) {
		t.Fatal("remix matched original")
	}
}
