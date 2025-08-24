package genre

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/spotify"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

// GenreHandler handles genre-based track searches
type GenreHandler struct {
	log               *zap.SugaredLogger
	spotifyClient     *spotify.SpotifyClient
	musicbrainzClient *musicbrainz.MusicbrainzClient
}

func (*GenreHandler) Pattern() string {
	return "/genre/tracks"
}

// NewGenreHandler builds a new GenreHandler
func NewGenreHandler(log *zap.SugaredLogger, spotifyClient *spotify.SpotifyClient, musicbrainzClient *musicbrainz.MusicbrainzClient) *GenreHandler {
	return &GenreHandler{
		log:               log,
		spotifyClient:     spotifyClient,
		musicbrainzClient: musicbrainzClient,
	}
}

type GenreRequest struct {
	Genre string `json:"genre"`
	Limit int    `json:"limit"`
}

type GenreResponse struct {
	Genre  string       `json:"genre"`
	Tracks []GenreTrack `json:"tracks"`
	Note   string       `json:"note,omitempty"`
}

type GenreTrack struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artist      string   `json:"artist"`
	Album       string   `json:"album"`
	Image       *string  `json:"image"`
	Popularity  int      `json:"popularity"`
	Genres      []string `json:"genres"`
	ISRC        string   `json:"isrc"`
	ReleaseDate string   `json:"release_date"`
	MBID        string   `json:"mbid,omitempty"`
}

// Search for tracks by genre on Spotify and enrich with MusicBrainz data
// @Summary Search tracks by genre
// @Description Search for tracks on Spotify by genre and enrich with MusicBrainz data
// @Tags Genre
// @Accept json
// @Produce json
// @Param request body GenreRequest true "Genre search request"
// @Success 200 {object} GenreResponse
// @Router /genre/tracks [post]
func (h *GenreHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	var req GenreRequest
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Genre == "" {
		http.Error(w, "missing genre", http.StatusBadRequest)
		return
	}

	// Spotify API limits: 1-50 for track search
	if req.Limit <= 0 {
		req.Limit = 20 // Default limit
	} else if req.Limit > 50 {
		req.Limit = 50 // Max limit for Spotify
	}

	h.log.Infow("genre search", "genre", req.Genre, "limit", req.Limit)

	// Search Spotify for tracks by genre with better query strategy
	var query string
	switch strings.ToLower(req.Genre) {
	case "rap", "hip-hop", "hip hop":
		// For rap/hip-hop, use a more targeted query that tends to return popular tracks
		query = "genre:rap tag:hip-hop"
	case "rock":
		query = "genre:rock tag:rock"
	case "pop":
		query = "genre:pop tag:pop"
	case "electronic", "edm":
		query = "genre:electronic tag:electronic"
	default:
		query = "genre:" + req.Genre
	}

	// Search with popularity boost - use year filter and market targeting for better results
	searchQuery := query + " year:2020-2024"
	results, err := h.spotifyClient.Client.Search(ctx, searchQuery, spot.SearchTypeTrack, spot.Limit(req.Limit), spot.Market("US"))
	if err != nil {
		h.log.Errorw("spotify search error", "error", err, "genre", req.Genre)
		http.Error(w, "spotify search error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var resp GenreResponse
	resp.Genre = req.Genre

	if results.Tracks != nil {
		// Collect ISRCs for bulk enrichment
		var isrcsToEnrich []string
		var tracksToEnrich []*GenreTrack

		for i, item := range results.Tracks.Tracks {
			track := h.mapSpotifyTrack(item)

			// Only enrich the first few tracks to avoid overwhelming MusicBrainz
			if i < 5 && track.ISRC != "" {
				isrcsToEnrich = append(isrcsToEnrich, track.ISRC)
				tracksToEnrich = append(tracksToEnrich, &track)
			}

			resp.Tracks = append(resp.Tracks, track)
		}

		// Bulk enrich tracks with ISRCs
		enrichmentCount := 0
		if len(isrcsToEnrich) > 0 {
			enrichmentCount = h.bulkEnrichWithMusicBrainz(ctx, tracksToEnrich, isrcsToEnrich)
			h.log.Infow("bulk enrichment completed", "tracks_enriched", enrichmentCount, "total_isrcs", len(isrcsToEnrich))
		}

		// Log popularity info for debugging
		if len(resp.Tracks) > 0 {
			avgPopularity := 0
			maxPopularity := 0
			for _, track := range resp.Tracks {
				avgPopularity += track.Popularity
				if track.Popularity > maxPopularity {
					maxPopularity = track.Popularity
				}
			}
			avgPopularity = avgPopularity / len(resp.Tracks)
			h.log.Infow("popularity stats", "avg_popularity", avgPopularity, "max_popularity", maxPopularity, "min_popularity", resp.Tracks[len(resp.Tracks)-1].Popularity)
		}

		h.log.Infow("genre search completed", "genre", req.Genre, "total_tracks", len(resp.Tracks), "enriched_tracks", enrichmentCount, "note", "limited enrichment to respect MusicBrainz rate limits")

		// Add a note about enrichment
		if enrichmentCount > 0 {
			resp.Note = fmt.Sprintf("Enhanced %d tracks with additional MusicBrainz data", enrichmentCount)
		}

		// Sort by popularity (descending order)
		sort.Slice(resp.Tracks, func(i, j int) bool {
			return resp.Tracks[i].Popularity > resp.Tracks[j].Popularity
		})
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *GenreHandler) mapSpotifyTrack(t spot.FullTrack) GenreTrack {
	var track GenreTrack

	track.ID = string(t.ID)
	track.Name = t.Name
	track.Artist = getFirstArtist(t.Artists)
	track.Album = t.Album.Name
	track.Popularity = int(t.Popularity)
	track.ReleaseDate = t.Album.ReleaseDate

	// Get album image
	if len(t.Album.Images) > 0 {
		imageURL := t.Album.Images[0].URL
		track.Image = &imageURL
	}

	// Get ISRC if available
	if len(t.ExternalIDs) > 0 {
		if isrc, ok := t.ExternalIDs["isrc"]; ok {
			track.ISRC = isrc
		}
	}

	// Note: SimpleAlbum doesn't have Genres field, we'll get genres from MusicBrainz enrichment

	return track
}

func (h *GenreHandler) enrichWithMusicBrainz(ctx context.Context, track *GenreTrack) bool {
	// Try to find MusicBrainz data by ISRC first
	if track.ISRC != "" {
		mbResp, err := h.musicbrainzClient.Client.SearchRecordingsByISRC(mb.SearchRecordingsByISRCRequest{
			ISRC: track.ISRC,
		})
		if err != nil {
			h.log.Debugw("MusicBrainz ISRC search failed", "isrc", track.ISRC, "error", err)
		} else if mbResp.Count > 0 {
			// Found by ISRC, use the first result
			mbTrack := mbResp.Recordings[0]
			track.MBID = mbTrack.ID

			// Add MusicBrainz genres if available
			if mbTrack.Genres != nil && len(*mbTrack.Genres) > 0 {
				for _, genre := range *mbTrack.Genres {
					// Add genre if not already present
					found := false
					for _, existingGenre := range track.Genres {
						if existingGenre == genre.Name {
							found = true
							break
						}
					}
					if !found {
						track.Genres = append(track.Genres, genre.Name)
					}
				}
			}
			return true
		}
	}

	// Fallback: search by artist and track name (only if we have both artist and track)
	if track.Artist != "" && track.Name != "" {
		mbResp, err := h.musicbrainzClient.Client.SearchRecordingsByArtistAndTrack(mb.SearchRecordingsByArtistAndTrackRequest{
			Artist: track.Artist,
			Track:  track.Name,
		})
		if err != nil {
			h.log.Debugw("MusicBrainz artist/track search failed", "artist", track.Artist, "track", track.Name, "error", err)
		} else if mbResp.Count > 0 {
			// Found by artist/track, use the first result
			mbTrack := mbResp.Recordings[0]
			track.MBID = mbTrack.ID

			// Add MusicBrainz genres if available
			if mbTrack.Genres != nil && len(*mbTrack.Genres) > 0 {
				for _, genre := range *mbTrack.Genres {
					// Add genre if not already present
					found := false
					for _, existingGenre := range track.Genres {
						if existingGenre == genre.Name {
							found = true
							break
						}
					}
					if !found {
						track.Genres = append(track.Genres, genre.Name)
					}
				}
			}
			return true
		}
	}

	// No enrichment happened
	return false
}

func (h *GenreHandler) bulkEnrichWithMusicBrainz(ctx context.Context, tracks []*GenreTrack, isrcs []string) int {
	if len(isrcs) == 0 || len(tracks) == 0 {
		return 0
	}

	h.log.Infow("starting bulk enrichment", "isrcs_count", len(isrcs), "tracks_count", len(tracks))

	// Use the new bulk ISRC search method
	mbResp, err := h.musicbrainzClient.Client.SearchRecordingsByBulkISRC(mb.SearchRecordingsByBulkISRCRequest{
		ISRCs: isrcs,
	})
	if err != nil {
		h.log.Warnw("bulk MusicBrainz enrichment failed", "error", err)
		return 0
	}

	if mbResp.Count == 0 {
		h.log.Infow("no MusicBrainz recordings found for bulk ISRC search")
		return 0
	}

	h.log.Infow("bulk enrichment found recordings", "recordings_count", mbResp.Count)

	// Map the results back to tracks using the ISRC mapping
	enrichedCount := 0
	for _, track := range tracks {
		if track.ISRC == "" {
			continue
		}

		// Check if we have recordings for this ISRC
		if recordings, exists := mbResp.ISRCMap[track.ISRC]; exists && len(recordings) > 0 {
			// Use the first recording found
			mbTrack := recordings[0]
			track.MBID = mbTrack.ID

			// Add MusicBrainz genres if available
			if mbTrack.Genres != nil && len(*mbTrack.Genres) > 0 {
				for _, genre := range *mbTrack.Genres {
					// Add genre if not already present
					found := false
					for _, existingGenre := range track.Genres {
						if existingGenre == genre.Name {
							found = true
							break
						}
					}
					if !found {
						track.Genres = append(track.Genres, genre.Name)
					}
				}
			}
			enrichedCount++
		}
	}

	h.log.Infow("bulk enrichment completed", "tracks_enriched", enrichedCount, "total_tracks", len(tracks))
	return enrichedCount
}

func getFirstArtist(artists []spot.SimpleArtist) string {
	if len(artists) > 0 {
		return artists[0].Name
	}
	return ""
}
