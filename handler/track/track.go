package track

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

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
		"acoustic bass guitar":     "bass",
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
	httpClient        *http.Client
	spotifyClient     *spotify.SpotifyClient
	musicbrainzClient *musicbrainz.MusicbrainzClient
}

func (*GetTrackHandler) Pattern() string {
	return "/track"
}

// NewGetTrackHandler builds a new GetTrackHandler.
func NewGetTrackHandler(
	log *zap.SugaredLogger,
	httpClient *http.Client,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
) *GetTrackHandler {
	return &GetTrackHandler{
		log:               log,
		httpClient:        httpClient,
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
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	mbid := q.Get("mbid")

	l := h.log

	var resp GetTrackResponse

	// V3 Version based on MBID, MusicBrainz recording ID
	if mbid != "" {
		l.Infow("Fetching MusicBrainz recording", "mbid", mbid)
		recording, err := h.musicbrainzClient.Client.GetRecording(mb.GetRecordingRequest{
			ID: mbid,
			Includes: []mb.Include{
				"artist-credits",
				"artist-rels",
				"genres",
				"releases",
				"work-rels",
				"url-rels",
			},
		})
		if err != nil {
			l.Errorf("error fetching recording: %v", err)
		}

		track := occipital.Track{
			ID:                mbid,
			Name:              recording.Recording.Title,
			Artist:            util.GetArtistCreditsFromRecording(*recording.Recording.ArtistCredits),
			ReleaseDate:       recording.Recording.FirstReleaseDate,
			Image:             getLatestReleaseImageURL(h.log, h.httpClient, recording.Recording),
			Instruments:       getArtistInstrumentsForRecording(recording.Recording),
			ProductionCredits: getProductionCreditsForRecording(recording.Recording),
			Genres:            getGenresForRecording(recording.Recording),
			Links:             getExternalLinksForRecording(recording.Recording),
			Releases:          getReleasesFromRecording(h.log, h.httpClient, recording.Recording),
		}

		work := h.getWorkFromRecording(recording.Recording)
		if work != nil {
			track.SongCredits = getSongCreditsForWork(*work)
		}

		resp.Track = track
		json.NewEncoder(w).Encode(resp)
		return
	}
}

func getArtistInstrumentsForRecording(rec mb.Recording) []*occipital.TrackInstrumentArtists {
	instrumentMap := make(map[string]map[string]struct{})

	// Iterate over relations and group artists by instrument
	for _, relation := range *rec.Relations {
		if relation.Type == "instrument" && len(relation.Attributes) == 1 {
			instrumentName := relation.Attributes[0]
			artistName := relation.Artist.Name

			if mappedInstrumentName, ok := instrumentMappings[instrumentName]; ok {
				instrumentName = mappedInstrumentName
			}

			if instrumentMap[instrumentName] == nil {
				instrumentMap[instrumentName] = make(map[string]struct{})
			}

			instrumentMap[instrumentName][artistName] = struct{}{}
		}
	}

	var instrumentArtists []*occipital.TrackInstrumentArtists
	for instrument, artistSet := range instrumentMap {
		artists := make([]string, 0, len(artistSet))
		for artist := range artistSet {
			artists = append(artists, artist)
		}

		sort.Strings(artists)

		instrumentArtists = append(instrumentArtists, &occipital.TrackInstrumentArtists{
			Instrument: instrument,
			Artists:    artists,
		})
	}

	// Sort the instruments by instrumentRankings (lower is higher priority), then by number of artists (descending)
	sort.Slice(instrumentArtists, func(i, j int) bool {
		rankI, okI := instrumentRankings[instrumentArtists[i].Instrument]
		rankJ, okJ := instrumentRankings[instrumentArtists[j].Instrument]

		// If both have a ranking, sort by ranking
		if okI && okJ {
			if rankI != rankJ {
				return rankI < rankJ
			}
		} else if okI {
			// Only i has a ranking, so it comes first
			return true
		} else if okJ {
			// Only j has a ranking, so j comes first
			return false
		}

		// If both have no ranking or same ranking, sort by number of artists (descending)
		if len(instrumentArtists[i].Artists) != len(instrumentArtists[j].Artists) {
			return len(instrumentArtists[i].Artists) > len(instrumentArtists[j].Artists)
		}

		// If still tied, sort alphabetically by instrument name
		return instrumentArtists[i].Instrument < instrumentArtists[j].Instrument
	})

	return instrumentArtists
}

func getGenresForRecording(rec mb.Recording) []string {
	maxGenres := 10
	genres := make([]string, 0, maxGenres)

	// First, try recording genres
	if rec.Genres != nil && len(*rec.Genres) > 0 {
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

	// If no genres found on recording, fall back to artist genres
	if len(genres) == 0 && rec.ArtistCredits != nil {
		genreCount := make(map[string]int)
		for _, credit := range *rec.ArtistCredits {
			if credit.Artist != nil && credit.Artist.Genres != nil {
				for _, g := range *credit.Artist.Genres {
					genreCount[g.Name] += g.Count
				}
			}
		}
		// Convert map to slice and sort by count
		var genreList []struct {
			Name  string
			Count int
		}
		for name, count := range genreCount {
			genreList = append(genreList, struct {
				Name  string
				Count int
			}{name, count})
		}
		sort.Slice(genreList, func(i, j int) bool {
			return genreList[i].Count > genreList[j].Count
		})
		for i := 0; i < maxGenres && i < len(genreList); i++ {
			genres = append(genres, genreList[i].Name)
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

func getLatestReleaseImageURL(l *zap.SugaredLogger, client *http.Client, recording mb.Recording) string {
	if recording.Releases == nil || len(*recording.Releases) == 0 {
		return ""
	}
	firstRelease := (*recording.Releases)[0]
	if firstRelease.ID == "" {
		return ""
	}
	url := fmt.Sprintf("https://coverartarchive.org/release/%s", firstRelease.ID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	var caaResp struct {
		Images []struct {
			Front      bool              `json:"front"`
			Thumbnails map[string]string `json:"thumbnails"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&caaResp); err != nil {
		return ""
	}
	for _, img := range caaResp.Images {
		if img.Front {
			if url500, ok := img.Thumbnails["500"]; ok {
				return url500
			}
		}
	}
	return ""
}

func getLatestReleaseMBID(client *http.Client, recording mb.Recording) string {
	// Return early if there are no releases to check.
	if recording.Releases == nil || len(*recording.Releases) == 0 {
		return ""
	}

	var bestReleaseID string
	var bestTime time.Time
	var hasFoundReleaseWithImage bool

	// Create a channel to receive the results from the goroutines.
	results := make(chan struct {
		releaseID string
		hasImage  bool
		time      time.Time
	})

	// Use a WaitGroup to wait for all goroutines to finish.
	var wg sync.WaitGroup

	// Iterate through each release associated with the recording.
	for _, release := range *recording.Releases {
		// A release must have an ID to be useful for cover art
		if release.ID == "" {
			continue
		}

		wg.Add(1)
		go func(release mb.Release) {
			defer wg.Done()

			// Try parsing the date in different formats
			var parsedTime time.Time
			var err error
			dateFormats := []string{"2006-01-02", "2006-01", "2006"}

			for _, format := range dateFormats {
				parsedTime, err = time.Parse(format, release.Date)
				if err == nil {
					break
				}
			}

			if err != nil {
				// Skip releases with invalid dates
				return
			}

			// Check if this release has an image by making a HEAD request to Cover Art Archive
			imageURL := getCoverArtArchiveImageURL(release.ID, "front", 500)

			// Use OPTIONS to check if the resource exists and supports GET/HEAD
			req, err := http.NewRequest("HEAD", imageURL.String(), nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
			resp, err := client.Do(req)
			hasImage := err == nil && resp != nil && resp.StatusCode == http.StatusOK
			if resp != nil {
				resp.Body.Close()
			}

			results <- struct {
				releaseID string
				hasImage  bool
				time      time.Time
			}{
				releaseID: release.ID,
				hasImage:  hasImage,
				time:      parsedTime,
			}
		}(release)
	}

	// Close the results channel when all goroutines are finished.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process the results from the channel.
	for result := range results {
		// If we haven't found a release with an image yet, or this one has an image
		if !hasFoundReleaseWithImage || result.hasImage {
			// If this is the first valid release found, or it's newer than the current best
			if bestReleaseID == "" || result.time.After(bestTime) {
				bestReleaseID = result.releaseID
				bestTime = result.time
				hasFoundReleaseWithImage = result.hasImage
			}
		}
	}

	return bestReleaseID
}

func getExternalLinksForRecording(rec mb.Recording) []occipital.ExternalLink {
	var links []occipital.ExternalLink
	for _, rel := range *rec.Relations {
		if rel.TargetType == "url" {
			if strings.Contains(rel.URL.Resource, "spotify") {
				links = append(links, occipital.ExternalLink{
					Type: "spotify",
					URL:  rel.URL.Resource,
				})
			}
			if strings.Contains(rel.URL.Resource, "genius") {
				links = append(links, occipital.ExternalLink{
					Type: "genius",
					URL:  rel.URL.Resource,
				})
			}
		}
	}
	return links
}

func getReleasesFromRecording(l *zap.SugaredLogger, client *http.Client, rec mb.Recording) *[]occipital.Release {
	if rec.Releases == nil || rec.ArtistCredits == nil || len(*rec.ArtistCredits) == 0 {
		return nil
	}
	// Build a set of all artist IDs from the recording's artist-credits
	artistIDs := make(map[string]struct{})
	for _, ac := range *rec.ArtistCredits {
		if ac.Artist != nil {
			artistIDs[ac.Artist.ID] = struct{}{}
		}
	}
	var releasesMu sync.Mutex
	releases := make([]occipital.Release, 0, len(*rec.Releases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // limit to 10 concurrent requests

	for _, mbRelease := range *rec.Releases {
		if mbRelease.Status != "Official" {
			continue
		}
		// For each release, check if any of its artist-credits matches any in the set
		hasMatchingArtist := false
		if mbRelease.ArtistCredit != nil {
			for _, ac := range *mbRelease.ArtistCredit {
				if ac.Artist != nil {
					if _, ok := artistIDs[ac.Artist.ID]; ok {
						hasMatchingArtist = true
						break
					}
				}
			}
		}
		if !hasMatchingArtist {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		mbReleaseCopy := mbRelease
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			release := occipital.Release{
				ID:             mbReleaseCopy.ID,
				Date:           mbReleaseCopy.Date,
				Country:        mbReleaseCopy.Country,
				Title:          mbRelease.Title,
				Disambiguation: mbReleaseCopy.Disambiguation,
				Image:          getCoverArtArchiveImageURL(mbReleaseCopy.ID, "front", 250).String(),
				Images:         getReleaseImagesForRelease(l, client, mbReleaseCopy.ID),
			}
			releasesMu.Lock()
			releases = append(releases, release)
			releasesMu.Unlock()
		}()
	}
	wg.Wait()
	return &releases
}

// getCoverArtArchiveImageURL returns the URL for a release image from Cover Art Archive.
// style should be "front" or "back", and size should be 250, 500, or 1200.
func getCoverArtArchiveImageURL(releaseID string, style string, size int) *url.URL {
	if style != "front" && style != "back" {
		style = "front"
	}
	if size != 250 && size != 500 && size != 1200 {
		size = 500
	}
	urlStr := fmt.Sprintf("https://coverartarchive.org/release/%s/%s-%d.jpg", releaseID, style, size)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return parsedURL
}

// getReleaseImagesForRelease fetches all images for a given release from the Cover Art Archive.
func getReleaseImagesForRelease(l *zap.SugaredLogger, client *http.Client, releaseID string) *[]occipital.ReleaseImage {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s", releaseID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	var caaResp struct {
		Images []struct {
			ID         int64             `json:"id"`
			Types      []string          `json:"types"`
			Thumbnails map[string]string `json:"thumbnails"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&caaResp); err != nil {
		return nil
	}
	var images []occipital.ReleaseImage
	for _, img := range caaResp.Images {
		imgType := ""
		if len(img.Types) > 0 {
			imgType = img.Types[0]
		}
		images = append(images, occipital.ReleaseImage{
			ID:    img.ID,
			Type:  imgType,
			Image: fmt.Sprintf("https://coverartarchive.org/release/%s/%d-250.jpg", releaseID, img.ID),
		})
	}
	return &images
}
