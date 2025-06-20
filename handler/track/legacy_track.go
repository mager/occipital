package track

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/util"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

func (h *GetTrackHandler) GetTrackV2(w http.ResponseWriter, isrc string) {
	l := h.log
	var resp GetTrackResponse

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
				Includes: []mb.Include{"artist-credits", "genres", "work-rels", "releases"},
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
			if recording.Recording.ArtistCredits != nil {
				track.Artist = util.GetArtistCreditsFromRecording(*recording.Recording.ArtistCredits)
			} else {
				track.Artist = "Various Artists"
			}
			track.Image = getLatestReleaseMBIDV0(recording.Recording)

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
}

func (h *GetTrackHandler) GetTrackV1(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	q := r.URL.Query()
	sourceId := q.Get("sourceId")
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
		l.Warn("API call failed", zap.Error(e))
	}

	// Add null checks before using the results
	if fullTrack == nil {
		l.Warn("Failed to fetch track data")
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
