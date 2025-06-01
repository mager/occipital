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
		"drum machine":             "drum-machine",
		"Wurlitzer electric piano": "wurlitzer",
		"electric piano":           "piano",
		"Rhodes piano":             "piano",
		"Minimoog":                 "synthesizer",
		"Moog":                     "synthesizer",
		"electronic instruments":   "synthesizer",
		"sampler":                  "synthesizer",
		"tenor saxophone":          "saxophone",
		"baritone saxophone":       "saxophone",
		"fretless bass":            "fretless-bass",
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
	ISRC     string `json:"isrc"`
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
	isrc := q.Get("isrc")

	l := h.log

	var resp GetTrackResponse
	var err error

	// V2 Version based on ISRC, start wtih Musicbrainz, then Spotify
	if isrc != "" {
		track := occipital.Track{
			ISRC: isrc,
		}
		searchRecsReq := mb.SearchRecordingsByISRCRequest{
			ISRC: isrc,
		}
		recs, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(searchRecsReq)
		if err != nil {
			l.Errorf("error fetching recordings: %v", err)
		}

		var recording mb.GetRecordingResponse
		if recs.Count >= 1 {
			getRecReq := mb.GetRecordingRequest{
				ID:       recs.Recordings[0].ID,
				Includes: []mb.Include{"artist-credits", "genres", "work-rels"},
			}
			recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
			if err != nil {
				l.Errorf("error fetching recording: %v", err)

				// Attempt to fetch the second recording if available
				if len(recs.Recordings) > 1 {
					getRecReq.ID = recs.Recordings[1].ID
					recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
					if err != nil {
						l.Errorf("error fetching second recording: %v", err)
					}
				}
			}

			if len(*recording.Relations) == 0 && len(recs.Recordings) > 1 {
				getRecReq.ID = recs.Recordings[1].ID
				recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
				if err != nil {
					l.Errorf("error fetching second recording: %v", err)
				}
			}

			l.Infow("Recording", "recording", recording)

			track.Name = recording.Recording.Title
			// track.Artist = util.GetFirstArtist(recording.Recording.Artists)
			// track.Image = *util.GetThumb(recording.Recording.Album)
			// track.ReleaseDate = *util.GetReleaseDate(recording.Recording.Album)
			track.Instruments = getArtistInstrumentsForRecording(recording.Recording)
			track.ProductionCredits = getProductionCreditsForRecording(recording.Recording)
			track.Genres = getGenresForRecording(recording.Recording)

			// If a work exists, get the song credits
			work := h.getWorkFromRecording(recording.Recording)
			if work != nil {
				track.SongCredits = getSongCreditsForWork(*work)
			}
		}

		resp.Track = track

		json.NewEncoder(w).Encode(resp)

		return
	}

	// V1 Version

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

	track := mapInitialTrack(r, fullTrack)
	if track.ISRC == "" {
		resp.Track = track
		json.NewEncoder(w).Encode(resp)
	}
	artistIDs := make([]spot.ID, 0, len(fullTrack.Artists))
	for _, artist := range fullTrack.Artists {
		artistIDs = append(artistIDs, spot.ID(artist.ID))
	}
	artists, err := h.spotifyClient.Client.GetArtists(ctx, artistIDs...)
	if err != nil {
		l.Errorf("error fetching artist: %v", err)
	}
	track.Genres = util.GetGenresForArtists(artists)

	// Call Musicbrainz to get the list of instruments for the track
	searchRecsReq := mb.SearchRecordingsByISRCRequest{
		ISRC: track.ISRC,
	}
	recs, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(searchRecsReq)
	if err != nil {
		l.Errorf("error fetching recordings: %v", err)
	}

	// TODO: Log if there are more than 1
	var recording mb.GetRecordingResponse
	if recs.Count >= 1 {
		getRecReq := mb.GetRecordingRequest{
			ID:       recs.Recordings[0].ID,
			Includes: []mb.Include{"artist-rels", "genres", "work-rels"},
		}
		recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
		if err != nil {
			l.Errorf("error fetching recording: %v", err)

			// Attempt to fetch the second recording if available
			if len(recs.Recordings) > 1 {
				getRecReq.ID = recs.Recordings[1].ID
				recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
				if err != nil {
					l.Errorf("error fetching second recording: %v", err)
				}
			}
		}

		if len(*recording.Relations) == 0 && len(recs.Recordings) > 1 {
			getRecReq.ID = recs.Recordings[1].ID
			recording, err = h.musicbrainzClient.Client.GetRecording(getRecReq)
			if err != nil {
				l.Errorf("error fetching second recording: %v", err)
			}
		}

		for _, relation := range *recording.Relations {
			l.Debugw("got relation", zap.Any("relation", relation))
		}

		track.Instruments = getArtistInstrumentsForRecording(recording.Recording)
		track.ProductionCredits = getProductionCreditsForRecording(recording.Recording)
		track.Genres = getGenresForRecording(recording.Recording)

		// If a work exists, get the song credits

		work := h.getWorkFromRecording(recording.Recording)
		if work != nil {
			track.SongCredits = getSongCreditsForWork(*work)
		}
	}

	resp.Track = track

	json.NewEncoder(w).Encode(resp)
}

func getArtistInstrumentsForRecording(rec mb.Recording) []*occipital.TrackInstrumentArtists {
	// Create a map to group artists by instrument
	instrumentMap := make(map[string]map[string]struct{})

	// Iterate over relations and group artists by instrument
	for _, relation := range *rec.Relations {
		if relation.Type == "instrument" && len(relation.Attributes) == 1 {
			instrumentName := relation.Attributes[0]
			artistName := relation.Artist.Name

			// Check if there's a mapping for the instrument
			if mappedInstrumentName, ok := instrumentMappings[instrumentName]; ok {
				instrumentName = mappedInstrumentName
			}

			// Initialize the artist set for this instrument if not already initialized
			if instrumentMap[instrumentName] == nil {
				instrumentMap[instrumentName] = make(map[string]struct{})
			}

			// Add artist to the instrument map (avoiding duplicates)
			instrumentMap[instrumentName][artistName] = struct{}{}
		}
	}

	// Convert instrumentMap to a slice of TrackInstrumentArtists
	var instrumentArtists []*occipital.TrackInstrumentArtists
	for instrument, artistSet := range instrumentMap {
		// Convert the set of artists to a slice
		artists := make([]string, 0, len(artistSet))
		for artist := range artistSet {
			artists = append(artists, artist)
		}

		// Sort the artists alphabetically
		sort.Strings(artists)

		// Create a TrackInstrumentArtists struct
		instrumentArtists = append(instrumentArtists, &occipital.TrackInstrumentArtists{
			Instrument: instrument,
			Artists:    artists,
		})
	}

	// Sort the instruments by the number of artists (descending)
	sort.Slice(instrumentArtists, func(i, j int) bool {
		return len(instrumentArtists[i].Artists) > len(instrumentArtists[j].Artists)
	})

	return instrumentArtists
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

func getProductionCreditsForRecording(rec mb.Recording) []*occipital.TrackProductionCredit {
	creditMap := make(map[string][]string)

	supportedTypes := []string{"producer", "mix", "recording", "vocal"}

	for _, relation := range *rec.Relations {
		for _, supportedType := range supportedTypes {
			if relation.Type == supportedType {
				creditMap[supportedType] = append(creditMap[supportedType], relation.Artist.Name)
			}
		}
	}

	var productionCredits []*occipital.TrackProductionCredit
	for creditType, artists := range creditMap {
		uniqueArtists := uniqueStrings(artists)

		// Append a new TrackProductionCredit with the creditType and unique artists
		productionCredits = append(productionCredits, &occipital.TrackProductionCredit{
			Credit:  creditType,
			Artists: uniqueArtists,
		})
	}

	// Sort the production credits by the number of artists (descending)
	sort.Slice(productionCredits, func(i, j int) bool {
		return len(productionCredits[i].Artists) > len(productionCredits[j].Artists)
	})

	return productionCredits
}

// Helper function to remove duplicate strings from a slice
func uniqueStrings(strs []string) []string {
	seen := make(map[string]struct{})
	var unique []string
	for _, str := range strs {
		if _, exists := seen[str]; !exists {
			seen[str] = struct{}{}
			unique = append(unique, str)
		}
	}
	return unique
}

func getSongCreditsForWork(rec mb.Work) []*occipital.TrackSongCredit {
	creditMap := make(map[string][]string)

	supportedTypes := []string{"composer", "lyricist", "writer"}
	for _, relation := range *rec.Relations {
		for _, supportedType := range supportedTypes {
			if relation.Type == supportedType {
				creditMap[supportedType] = append(creditMap[supportedType], relation.Artist.Name)
			}
		}
	}

	var songCredits []*occipital.TrackSongCredit
	for creditType, artists := range creditMap {
		uniqueArtists := uniqueStrings(artists)

		// Append a new TrackSongCredit with the creditType and unique artists
		songCredits = append(songCredits, &occipital.TrackSongCredit{
			Credit:  creditType,
			Artists: uniqueArtists,
		})
	}

	return songCredits
}

func (h *GetTrackHandler) getWorkFromRecording(rec mb.Recording) *mb.Work {
	for _, relation := range *rec.Relations {
		if relation.TargetType == "work" {
			work, err := h.musicbrainzClient.Client.GetWork(mb.GetWorkRequest{
				ID:       relation.Work.ID,
				Includes: []mb.Include{"artist-rels", "url-rels"},
			})
			if err != nil {
				h.log.Errorf("error fetching work: %v", err)
				return nil
			}
			return &work.Work
		}
	}

	return nil
}

func mapInitialTrack(r *http.Request, ft *spot.FullTrack) occipital.Track {
	var track occipital.Track
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
	source := q.Get("source")

	track.Name = ft.Name
	track.Artist = util.GetFirstArtist(ft.Artists)
	track.SourceID = sourceId
	track.Source = source
	track.Image = *util.GetThumb(ft.Album)
	track.ReleaseDate = *util.GetReleaseDate(ft.Album)
	track.ISRC = *util.GetISRC(ft)
	return track
}
