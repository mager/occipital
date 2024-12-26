package track

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"

	spot "github.com/zmb3/spotify/v2"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

var (
	instrumentMappings = map[string]string{
		"electric bass guitar":     "bass",
		"bass guitar":              "bass",
		"drums (drum set)":         "drums",
		"percussion":               "drums",
		"acoustic guitar":          "guitar",
		"family guitar":            "guitar",
		"electric guitar":          "guitar",
		"foot stomps":              "foot-stomps",
		"double bass":              "double-bass",
		"Wurlitzer electric piano": "wurlitzer",
		"Rhodes piano":             "piano",
	}
	instrumentRankings = map[string]int{
		"piano":    1,
		"guitar":   2,
		"bass":     3,
		"keyboard": 4,
		"drums":    5,
	}
)

// GetTrackHandler is an http.Handler
type GetTrackHandler struct {
	log               *zap.SugaredLogger
	spotifyClient     *spotify.SpotifyClient
	musicbrainzClient *musicbrainz.MusicbrainzClient
}

func (*GetTrackHandler) Pattern() string {
	return "/track"
}

// NewGetTrackHandler builds a new GetTrackHandler.
func NewGetTrackHandler(
	log *zap.SugaredLogger,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
) *GetTrackHandler {
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
// @Param sourceId query string true "Source ID"
// @Param source query string true "Source"
func (h *GetTrackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
	source := q.Get("source")

	l := h.log

	var resp GetTrackResponse
	var err error

	// Channels for receiving results
	var wg sync.WaitGroup
	var fullTrack *spot.FullTrack
	errChan := make(chan error, 3)

	// Fetch track
	wg.Add(1)
	go func() {
		defer wg.Done()
		t, err := h.spotifyClient.Client.GetTrack(ctx, spot.ID(sourceId))
		if err != nil {
			errChan <- fmt.Errorf("error fetching track: %v", err)
			return
		}
		fullTrack = t
	}()

	// Wait for all requests to complete
	wg.Wait()

	// Check for errors from any of the goroutines
	close(errChan)
	for e := range errChan {
		// Log the error but continue processing
		h.log.Warn("API call failed", zap.Error(e))
	}

	// Add null checks before using the results
	if fullTrack == nil {
		h.log.Warn("Failed to fetch track data")
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	var track occipital.Track
	track.Name = fullTrack.Name
	track.Artist = util.GetFirstArtist(fullTrack.Artists)
	track.SourceID = sourceId
	track.Source = source
	track.Image = *util.GetThumb(fullTrack.Album)
	track.ReleaseDate = *util.GetReleaseDate(fullTrack.Album)

	artistIDs := make([]spot.ID, 0, len(fullTrack.Artists))
	for _, artist := range fullTrack.Artists {
		artistIDs = append(artistIDs, spot.ID(artist.ID))
	}
	artists, err := h.spotifyClient.Client.GetArtists(ctx, artistIDs...)
	if err != nil {
		l.Errorf("error fetching artist: %v", err)
	}

	track.Genres = util.GetGenresForArtists(artists)
	track.ISRC = *util.GetISRC(fullTrack)
	if track.ISRC == "" {
		resp.Track = track
		json.NewEncoder(w).Encode(resp)
	}

	// Call Musicbrainz to get the list of instruments for the track
	searchRecsReq := mb.SearchRecordingsByISRCRequest{
		ISRC: track.ISRC,
	}
	recs, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(searchRecsReq)
	if err != nil {
		l.Errorf("error fetching recordings: %v", err)
	}

	// TODO: Log if there are more than 1
	if recs.Count == 1 {
		getRecReq := mb.GetRecordingRequest{
			ID:       recs.Recordings[0].ID,
			Includes: []mb.Include{"artist-rels", "genres"},
		}
		rec, err := h.musicbrainzClient.Client.GetRecording(getRecReq)
		if err != nil {
			l.Errorf("error fetching recording: %v", err)
		}

		l.Debugw("got recording", zap.String("ID", rec.ID), zap.String("ISRC", track.ISRC))

		for _, relation := range *rec.Relations {
			l.Debugw("got relation", zap.Any("relation", relation))
		}

		track.Instruments = getArtistInstrumentsForRecording(rec.Recording)
		track.ProductionCredits = getProductionCreditsForRecording(rec.Recording)
		track.Genres = getGenresForRecording(rec.Recording)
	}

	resp.Track = track

	json.NewEncoder(w).Encode(resp)
}

func getArtistInstrumentsForRecording(rec mb.Recording) []*occipital.TrackArtistInstruments {
	artistInstrumentMap := make(map[string][]string)

	for _, relation := range *rec.Relations {
		if relation.Type == "instrument" && len(relation.Attributes) == 1 {
			// Get instrument name and artist name
			instrumentName := relation.Attributes[0]
			artistName := relation.Artist.Name

			// Check if there's a mapping for the instrument
			if mappedInstrumentName, ok := instrumentMappings[instrumentName]; ok {
				instrumentName = mappedInstrumentName
			}

			// Add instrument to artist map
			if _, ok := artistInstrumentMap[artistName]; !ok {
				artistInstrumentMap[artistName] = []string{instrumentName}
			} else {
				// Check if instrument already exists for this artist
				found := false
				for _, instrument := range artistInstrumentMap[artistName] {
					if instrument == instrumentName {
						found = true
						break
					}
				}
				if !found {
					artistInstrumentMap[artistName] = append(artistInstrumentMap[artistName], instrumentName)
				}
			}
		}
	}

	// Convert artistInstrumentMap to []*TrackArtistInstruments
	artistInstruments := make([]*occipital.TrackArtistInstruments, 0, len(artistInstrumentMap))
	for artistName, instruments := range artistInstrumentMap {
		// Sort the instruments alphabetically
		sort.Strings(instruments)

		// Sort the instruments based on the predefined rankings
		sort.SliceStable(instruments, func(i, j int) bool {
			rankI, okI := instrumentRankings[instruments[i]]
			rankJ, okJ := instrumentRankings[instruments[j]]
			if !okI {
				rankI = len(instrumentRankings) + 1
			}
			if !okJ {
				rankJ = len(instrumentRankings) + 1
			}
			return rankI < rankJ
		})

		artistInstruments = append(artistInstruments, &occipital.TrackArtistInstruments{
			Artist:      artistName,
			Instruments: instruments,
		})
	}

	// Sort artists by the number of instruments they play
	sort.Slice(artistInstruments, func(i, j int) bool {
		return len(artistInstruments[i].Instruments) > len(artistInstruments[j].Instruments)
	})

	return artistInstruments
}

func getGenresForRecording(rec mb.Recording) []string {
	maxGenres := 3
	genres := make([]string, 0, maxGenres)

	if rec.Genres != nil && len(*rec.Genres) > 0 {
		// Dereference the pointer before sorting
		genresSlice := *rec.Genres

		// Sort genres by Count in descending order
		sort.Slice(genresSlice, func(i, j int) bool {
			return genresSlice[i].Count > genresSlice[j].Count
		})

		// Add genres with the highest counts, up to the max limit
		for i := 0; i < maxGenres && i < len(genresSlice); i++ {
			genres = append(genres, genresSlice[i].Name)
		}
	}

	return genres
}
func getProductionCreditsForRecording(rec mb.Recording) []*occipital.TrackArtistProduction {
	artistCreditsMap := make(map[string][]string)

	supportedTypes := []string{"producer", "mix", "recording"}

	for _, relation := range *rec.Relations {
		for _, supportedType := range supportedTypes { // Check against supported types
			if relation.Type == supportedType {
				artistCreditsMap[relation.Artist.Name] = append(artistCreditsMap[relation.Artist.Name], relation.Type)
			}
		}
	}

	// Convert artistCreditsMap to []*TrackArtistProduction and sort by total number of credits
	artistCredits := make([]*occipital.TrackArtistProduction, 0, len(artistCreditsMap))
	for artistName, credits := range artistCreditsMap {
		sort.Strings(credits) // Sort the credits for each artist
		artistCredits = append(artistCredits, &occipital.TrackArtistProduction{
			Artist:  artistName,
			Credits: credits,
		})
	}

	sort.Slice(artistCredits, func(i, j int) bool {
		return len(artistCredits[i].Credits) > len(artistCredits[j].Credits)
	})

	return artistCredits
}

// SimplifySegments reduces the number of segments by averaging over a fixed interval
func SimplifySegments(segments []occipital.TrackAnalysisSegment, groupSize int) []occipital.TrackAnalysisSegment {
	simplifiedSegments := []occipital.TrackAnalysisSegment{}
	var currentGroup []occipital.TrackAnalysisSegment

	for i, segment := range segments {
		currentGroup = append(currentGroup, segment)

		// Once we have enough segments for the group, process the group
		if (i+1)%groupSize == 0 || i == len(segments)-1 {
			avgSegment := averageGroup(currentGroup)
			simplifiedSegments = append(simplifiedSegments, avgSegment)
			currentGroup = []occipital.TrackAnalysisSegment{} // Reset group
		}
	}
	return simplifiedSegments
}

// averageGroup calculates the average values for a group of segments
func averageGroup(group []occipital.TrackAnalysisSegment) occipital.TrackAnalysisSegment {
	totalDuration := 0.0
	totalConfidence := 0.0
	totalLoudnessMax := 0.0
	totalLoudnessStart := 0.0
	totalLoudnessEnd := 0.0

	for _, segment := range group {
		totalDuration += segment.Duration
		totalConfidence += segment.Confidence
		totalLoudnessMax += segment.LoudnessMax
		totalLoudnessStart += segment.LoudnessStart
		totalLoudnessEnd += segment.LoudnessEnd
	}

	groupSize := float64(len(group))
	return occipital.TrackAnalysisSegment{
		Duration:      totalDuration,
		Confidence:    totalConfidence / groupSize,
		LoudnessMax:   totalLoudnessMax / groupSize,
		LoudnessStart: totalLoudnessStart / groupSize,
		LoudnessEnd:   totalLoudnessEnd / groupSize,
		Start:         group[0].Start, // Use the start of the first segment
	}
}
