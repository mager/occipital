package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	spotifyLib "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

// GetCreatorHandler is an http.Handler
type GetCreatorHandler struct {
	log               *zap.SugaredLogger
	musicbrainzClient *musicbrainz.MusicbrainzClient
	spotifyClient     *spotify.SpotifyClient
}

func (*GetCreatorHandler) Pattern() string {
	return "/creator"
}

// NewGetCreatorHandler builds a new GetCreatorHandler.
func NewGetCreatorHandler(
	log *zap.SugaredLogger,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
	spotifyClient *spotify.SpotifyClient,
) *GetCreatorHandler {
	return &GetCreatorHandler{
		log:               log,
		musicbrainzClient: musicbrainzClient,
		spotifyClient:     spotifyClient,
	}
}

type GetCreatorResponse struct {
	Creator occipital.Creator `json:"creator"`
}

func (h *GetCreatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	mbid := q.Get("mbid")

	if mbid == "" {
		http.Error(w, `{"error":"mbid is required"}`, http.StatusBadRequest)
		return
	}

	h.log.Infow("Fetching MusicBrainz artist", "mbid", mbid)

	artistResp, err := h.musicbrainzClient.Client.GetArtist(mb.GetArtistRequest{
		ID: mbid,
		Includes: []mb.Include{
			"genres",
			"url-rels",
			"recording-rels",
			"artist-credits",
		},
	})
	if err != nil {
		h.log.Errorf("error fetching artist: %v", err)
		http.Error(w, `{"error":"failed to fetch artist"}`, http.StatusInternalServerError)
		return
	}

	creator := transformArtistToCreator(artistResp.Artist)

	// Fetch Spotify highlights
	highlights := h.fetchHighlights(creator.Links)
	if len(highlights) > 0 {
		creator.Highlights = highlights
	}

	resp := GetCreatorResponse{Creator: creator}
	json.NewEncoder(w).Encode(resp)
}

// fetchHighlights extracts the Spotify artist ID from links and fetches top tracks.
func (h *GetCreatorHandler) fetchHighlights(links []occipital.ExternalLink) []occipital.CreatorHighlight {
	spotifyID := extractSpotifyArtistID(links)
	if spotifyID == "" {
		return nil
	}

	h.log.Infow("Fetching Spotify top tracks", "spotifyArtistID", spotifyID)

	ctx := context.Background()
	topTracks, err := h.spotifyClient.Client.GetArtistsTopTracks(ctx, spotifyLib.ID(spotifyID), "US")
	if err != nil {
		h.log.Warnw("Failed to fetch Spotify top tracks", "err", err)
		return nil
	}

	var highlights []occipital.CreatorHighlight
	for _, track := range topTracks {
		image := ""
		if track.Album.Images != nil && len(track.Album.Images) > 0 {
			image = track.Album.Images[0].URL
		}

		// Build artist string
		var artists []string
		for _, a := range track.Artists {
			artists = append(artists, a.Name)
		}

		highlights = append(highlights, occipital.CreatorHighlight{
			ID:     string(track.ID),
			Title:  track.Name,
			Artist: strings.Join(artists, ", "),
			Image:  image,
		})
	}

	// Cap at 10 highlights
	if len(highlights) > 10 {
		highlights = highlights[:10]
	}

	return highlights
}

// extractSpotifyArtistID finds the Spotify artist ID from MusicBrainz URL relations.
// Looks for URLs like https://open.spotify.com/artist/XXXXX
func extractSpotifyArtistID(links []occipital.ExternalLink) string {
	for _, link := range links {
		if strings.Contains(link.URL, "open.spotify.com/artist/") {
			parts := strings.Split(link.URL, "/artist/")
			if len(parts) == 2 {
				// Strip any query params
				id := parts[1]
				if idx := strings.Index(id, "?"); idx > 0 {
					id = id[:idx]
				}
				return id
			}
		}
	}
	return ""
}

func transformArtistToCreator(artist mb.Artist) occipital.Creator {
	creator := occipital.Creator{
		ID:             artist.ID,
		Name:           artist.Name,
		Type:           artist.Type,
		Disambiguation: artist.Disambiguation,
		Country:        artist.Country,
		Genres:         extractGenres(artist),
		Links:          extractLinks(artist),
		Credits:        extractCredits(artist),
	}

	if artist.Area != nil {
		creator.Area = artist.Area.Name
	}
	if artist.BeginArea != nil {
		creator.BeginArea = artist.BeginArea.Name
	}
	if artist.LifeSpan != nil {
		creator.ActiveYears = &occipital.ActiveYears{
			Begin: artist.LifeSpan.Begin,
			End:   artist.LifeSpan.End,
			Ended: artist.LifeSpan.Ended,
		}
	}

	return creator
}

func extractGenres(artist mb.Artist) []string {
	maxGenres := 10
	genres := make([]string, 0)

	if artist.Genres == nil || len(*artist.Genres) == 0 {
		return genres
	}

	genresSlice := *artist.Genres
	sort.Slice(genresSlice, func(i, j int) bool {
		return genresSlice[i].Count > genresSlice[j].Count
	})

	for i := 0; i < maxGenres && i < len(genresSlice); i++ {
		genres = append(genres, genresSlice[i].Name)
	}

	return genres
}

func extractLinks(artist mb.Artist) []occipital.ExternalLink {
	var links []occipital.ExternalLink

	if artist.Relations == nil {
		return links
	}

	supportedDomains := map[string]string{
		"spotify":    "spotify",
		"wikipedia":  "wikipedia",
		"bandcamp":   "bandcamp",
		"soundcloud": "soundcloud",
		"discogs":    "discogs",
		"allmusic":   "allmusic",
		"youtube":    "youtube",
		"instagram":  "instagram",
		"twitter":    "twitter",
		"facebook":   "facebook",
		"wikidata":   "wikidata",
	}

	for _, rel := range *artist.Relations {
		if rel.TargetType == "url" {
			for domain, linkType := range supportedDomains {
				if strings.Contains(rel.URL.Resource, domain) {
					links = append(links, occipital.ExternalLink{
						Type: linkType,
						URL:  rel.URL.Resource,
					})
					break
				}
			}
		}
	}

	return links
}

func extractCredits(artist mb.Artist) []occipital.CreatorCredit {
	if artist.Relations == nil {
		return nil
	}

	// Group recordings by credit type
	creditMap := make(map[string][]occipital.CreatorRecording)

	for _, rel := range *artist.Relations {
		if rel.TargetType == "recording" && rel.Recording != nil {
			creditType := rel.Type
			// For instrument relations, include the attribute (e.g., "guitar", "bass")
			if rel.Type == "instrument" && len(rel.Attributes) > 0 {
				creditType = rel.Attributes[0]
			}

			// Extract artist name from recording's artist-credit
			artist := ""
			if rel.Recording.ArtistCredits != nil {
				var parts []string
				for _, ac := range *rel.Recording.ArtistCredits {
					parts = append(parts, ac.Name+ac.JoinPhrase)
				}
				artist = strings.Join(parts, "")
			}

			creditMap[creditType] = append(creditMap[creditType], occipital.CreatorRecording{
				ID:     rel.Recording.ID,
				Title:  rel.Recording.Title,
				Artist: artist,
			})
		}
	}

	var credits []occipital.CreatorCredit
	for creditType, recordings := range creditMap {
		credits = append(credits, occipital.CreatorCredit{
			Type:       creditType,
			Recordings: recordings,
		})
	}

	// Sort credits by number of recordings (descending)
	sort.Slice(credits, func(i, j int) bool {
		return len(credits[i].Recordings) > len(credits[j].Recordings)
	})

	// Consolidate: keep top 10 credit types, merge the rest into "other"
	const maxCreditTypes = 10
	if len(credits) > maxCreditTypes {
		var otherRecordings []occipital.CreatorRecording
		seen := make(map[string]bool)
		for _, c := range credits[maxCreditTypes:] {
			for _, r := range c.Recordings {
				if !seen[r.ID] {
					seen[r.ID] = true
					otherRecordings = append(otherRecordings, r)
				}
			}
		}
		credits = credits[:maxCreditTypes]
		if len(otherRecordings) > 0 {
			credits = append(credits, occipital.CreatorCredit{
				Type:       fmt.Sprintf("other (%d types)", len(credits)),
				Recordings: otherRecordings,
			})
		}
	}

	return credits
}
