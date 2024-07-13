package track

import (
	"context"
	"encoding/json"
	"net/http"

	spot "github.com/zmb3/spotify/v2"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

// GetTrackHandler is an http.Handler
type GetTrackHandler struct {
	log               *zap.Logger
	spotifyClient     *spotify.SpotifyClient
	musicbrainzClient *musicbrainz.MusicbrainzClient
}

func (*GetTrackHandler) Pattern() string {
	return "/track"
}

// NewGetTrackHandler builds a new GetTrackHandler.
func NewGetTrackHandler(log *zap.Logger, spotifyClient *spotify.SpotifyClient, musicbrainzClient *musicbrainz.MusicbrainzClient) *GetTrackHandler {
	return &GetTrackHandler{
		log:               log,
		spotifyClient:     spotifyClient,
		musicbrainzClient: musicbrainzClient,
	}
}

type GetTrackRequest struct {
	SourceID string `json:"source_id"`
	Source   string `json:"source"`
}

type GetTrackResponse struct {
	Track occipital.Track `json:"track"`
}

// Get track
// @Summary Get track
// @Description Get track
// @Accept json
// @Produce json
// @Success 200 {object} GetTrackResponse
// @Router /track [get]
func (h *GetTrackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
	source := q.Get("source")

	var resp GetTrackResponse
	var err error

	// Channels for receiving results
	trackChan := make(chan *spot.FullTrack, 1)
	audioFeaturesChan := make(chan []*spot.AudioFeatures, 1)
	// Fetch track asynchronously
	go func() {
		var t *spot.FullTrack
		t, err = h.spotifyClient.Client.GetTrack(ctx, spot.ID(sourceId))
		trackChan <- t
		if err != nil {
			h.log.Sugar().Errorf("error fetching track: %v", err)
		}
	}()

	// Fetch audio features asynchronously
	go func() {
		var audioFeatures []*spot.AudioFeatures
		audioFeatures, err = h.spotifyClient.Client.GetAudioFeatures(ctx, spot.ID(sourceId))
		audioFeaturesChan <- audioFeatures
		if err != nil {
			h.log.Sugar().Errorf("error fetching audio features: %v", err)
		}
	}()

	// Receive track and audio features
	t := <-trackChan
	audioFeatures := <-audioFeaturesChan

	// Check for errors
	if err != nil {
		http.Error(w, "track fetch error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var track occipital.Track
	track.Name = t.Name
	track.Artist = util.GetFirstArtist(t.Artists)
	track.SourceID = sourceId
	track.Source = source
	track.Image = *util.GetThumb(t.Album)

	// Audio features
	if audioFeatures == nil || (len(audioFeatures) == 0 || len(audioFeatures) > 1) {
		h.log.Sugar().Warn("Error getting audio features", zap.Int("len_features", len(audioFeatures)))
	} else {
		af := audioFeatures[0]
		f := &occipital.TrackFeatures{
			Acousticness:     af.Acousticness,
			Danceability:     af.Danceability,
			DurationMs:       int(af.Duration),
			Energy:           af.Energy,
			Happiness:        af.Valence,
			Instrumentalness: af.Instrumentalness,
			Key:              int(af.Key),
			Liveness:         af.Liveness,
			Loudness:         af.Loudness,
			Mode:             int(af.Mode),
			Speechiness:      af.Speechiness,
			Tempo:            af.Tempo,
			TimeSignature:    int(af.TimeSignature),
		}
		track.Features = f
	}

	track.ReleaseDate = *util.GetReleaseDate(t.Album)

	artistIDs := make([]spot.ID, len(t.Artists))
	for _, artist := range t.Artists {
		artistIDs = append(artistIDs, spot.ID(artist.ID))
	}
	artists, err := h.spotifyClient.Client.GetArtists(ctx, artistIDs...)
	if err != nil {
		h.log.Sugar().Errorf("error fetching artist: %v", err)
	}
	track.Genres = util.GetGenresForArtists(artists)

	track.ISRC = *util.GetISRC(t)
	if track.ISRC != "" {
		// Call Musicbrainz to get the list of instruments for the track
		searchRecsReq := mb.SearchRecordingsByISRCRequest{
			ISRC: track.ISRC,
		}
		recs, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(searchRecsReq)
		if err != nil {
			h.log.Sugar().Errorf("error fetching recordings: %v", err)
		}
		// If there is a single recording, fetch it
		if recs.Count == 1 {
			getRecReq := mb.GetRecordingRequest{
				ID:       recs.Recordings[0].ID,
				Includes: []mb.Include{"artist-rels"},
			}
			rec, err := h.musicbrainzClient.Client.GetRecording(getRecReq)
			if err != nil {
				h.log.Sugar().Errorf("error fetching recording: %v", err)
			}
			h.log.Sugar().Infow("got recording", "ID", rec.ID)

			// Get instruments for track
			track.Instruments = getInstrumentsForRecording(rec.Recording)
		}
	}

	resp.Track = track

	json.NewEncoder(w).Encode(resp)
}
func getInstrumentsForRecording(rec mb.Recording) []*occipital.TrackInstrument {
	// Use a map to store instruments with their artists
	instrumentMap := make(map[string][]string)

	// Define instrument mappings for merging
	instrumentMappings := map[string]string{
		"bass guitar":      "bass",
		"drums (drum set)": "drums",
	}

	// Iterate through each relation
	for _, relation := range *rec.Relations {
		if relation.Type == "instrument" && len(relation.Attributes) == 1 {
			// Get instrument name and artist name
			instrumentName := relation.Attributes[0]
			artistName := relation.Artist.Name

			// Check if there's a mapping for the instrument
			mappedInstrumentName, ok := instrumentMappings[instrumentName]
			if ok {
				instrumentName = mappedInstrumentName
			}

			// Add artist to instrument map
			if _, ok := instrumentMap[instrumentName]; !ok {
				instrumentMap[instrumentName] = []string{artistName}
			} else {
				// Check if artist already exists for this instrument
				found := false
				for _, artist := range instrumentMap[instrumentName] {
					if artist == artistName {
						found = true
						break
					}
				}
				if !found {
					instrumentMap[instrumentName] = append(instrumentMap[instrumentName], artistName)
				}
			}
		}
	}

	// Convert instrumentMap to []*occipital.TrackInstrument
	ins := make([]*occipital.TrackInstrument, 0, len(instrumentMap))
	for instrumentName, artists := range instrumentMap {
		ins = append(ins, &occipital.TrackInstrument{
			Name:    instrumentName,
			Artists: artists,
		})
	}

	return ins
}